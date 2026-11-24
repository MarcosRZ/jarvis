package data

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"
)

// ErrBadTemplate marks a template that cannot be stored. It is a content
// error, not a write error, so the caller can turn it into a pending draft.
var ErrBadTemplate = errors.New("invalid data template")

// maxTemplateRunes bounds one template.
const maxTemplateRunes = 400

var selectorPattern = regexp.MustCompile(`^[a-z0-9_]+\.[a-z0-9_]+\.(attributes\.)?(state|name|unit|condition|temperature|humidity|wind_speed|pressure|battery_level|current_temperature|templow|precipitation|precipitation_probability|forecast\.(today|tomorrow|[0-9]+)\.(condition|temperature|templow|precipitation|precipitation_probability|wind_speed))$`)

// Render fills every placeholder of a template from the snapshot. It fails
// instead of speaking a placeholder or an English condition code.
func Render(template string, s Snapshot) (string, error) {
	var out strings.Builder
	rest := template
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			out.WriteString(rest)
			break
		}
		out.WriteString(rest[:open])
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			return "", fmt.Errorf("%w: unbalanced braces", ErrBadTemplate)
		}
		selector := strings.TrimSpace(rest[open+1 : open+close])
		value, err := resolveSelector(s, selector)
		if err != nil {
			return "", err
		}
		out.WriteString(value)
		rest = rest[open+close+1:]
	}
	text := collapseSpaces(out.String())
	if text == "" {
		return "", fmt.Errorf("%w: empty template", ErrBadTemplate)
	}
	return text, nil
}

// Validate checks that a template is structurally correct and that every
// placeholder can be resolved from the current snapshot. The snapshot check is
// what keeps an invented entity or attribute out of memory.
func Validate(template string, s Snapshot) error {
	template = strings.TrimSpace(template)
	if template == "" {
		return fmt.Errorf("%w: empty template", ErrBadTemplate)
	}
	if len([]rune(template)) > maxTemplateRunes {
		return fmt.Errorf("%w: template is longer than %d characters", ErrBadTemplate, maxTemplateRunes)
	}
	if !strings.Contains(template, "{") {
		return fmt.Errorf("%w: template has no placeholder", ErrBadTemplate)
	}
	switch last := []rune(template)[len([]rune(template))-1]; last {
	case '.', '?', '!':
	default:
		return fmt.Errorf("%w: template must end with a full stop, a question mark or an exclamation mark", ErrBadTemplate)
	}
	if strings.Contains(template, "  ") || strings.Contains(template, " ,") ||
		strings.Contains(template, " .") || strings.Contains(template, "..") {
		return fmt.Errorf("%w: template has broken spacing or punctuation", ErrBadTemplate)
	}
	rest := template
	placeholders := 0
	for {
		open := strings.IndexByte(rest, '{')
		if open < 0 {
			if strings.Contains(rest, "}") {
				return fmt.Errorf("%w: unbalanced braces", ErrBadTemplate)
			}
			break
		}
		if open > 0 {
			previous := []rune(rest[:open])[len([]rune(rest[:open]))-1]
			if unicode.IsLetter(previous) || unicode.IsDigit(previous) {
				return fmt.Errorf("%w: a placeholder must be a separate word", ErrBadTemplate)
			}
		}
		close := strings.IndexByte(rest[open:], '}')
		if close < 0 {
			return fmt.Errorf("%w: unbalanced braces", ErrBadTemplate)
		}
		selector := strings.TrimSpace(rest[open+1 : open+close])
		if !selectorPattern.MatchString(selector) {
			return fmt.Errorf("%w: unknown placeholder %q", ErrBadTemplate, selector)
		}
		if _, err := resolveSelector(s, selector); err != nil {
			return err
		}
		after := rest[open+close+1:]
		if after != "" {
			next := []rune(after)[0]
			if unicode.IsLetter(next) || unicode.IsDigit(next) {
				return fmt.Errorf("%w: a placeholder must be a separate word", ErrBadTemplate)
			}
		}
		placeholders++
		rest = after
	}
	if placeholders == 0 {
		return fmt.Errorf("%w: template has no placeholder", ErrBadTemplate)
	}
	for code := range englishConditions {
		if containsAny(normalize(template), strings.ReplaceAll(code, "-", " ")) {
			return fmt.Errorf("%w: template contains the untranslated condition %q", ErrBadTemplate, code)
		}
	}
	return nil
}

// TemplateFromAnswer turns a spoken answer that uses snapshot values into a
// template. It returns false when no current value appears in the answer, which
// means that the answer is a fixed reply rather than a datum.
func TemplateFromAnswer(q Question, answer string, s Snapshot) (string, bool) {
	answer = strings.TrimSpace(answer)
	if answer == "" {
		return "", false
	}
	candidates := templateCandidates(q, s)
	template := answer
	replaced := 0
	for _, candidate := range candidates {
		if candidate.text == "" {
			continue
		}
		pattern := regexp.MustCompile(`(?i)(^|[^a-z0-9])` + regexp.QuoteMeta(candidate.text) + `($|[^a-z0-9])`)
		if !pattern.MatchString(template) {
			continue
		}
		template = pattern.ReplaceAllString(template, "${1}{"+candidate.selector+"}${2}")
		replaced++
	}
	if replaced == 0 {
		return "", false
	}
	template = collapseSpaces(template)
	if err := Validate(template, s); err != nil {
		return "", false
	}
	return template, true
}

type candidate struct {
	text     string
	selector string
}

// templateCandidates lists the values that may appear in an answer to q, with
// the placeholder that replaces each one.
func templateCandidates(q Question, s Snapshot) []candidate {
	var out []candidate
	add := func(text, selector string) {
		text = strings.TrimSpace(text)
		if text != "" {
			out = append(out, candidate{text: text, selector: selector})
		}
	}
	weather, hasWeather := s.WeatherEntity()
	if hasWeather {
		addCondition := func(prefix, code string) {
			for _, phrase := range conditionVariants(code) {
				add(phrase, prefix+".condition")
			}
		}
		addCondition("weather."+entityObject(weather), weather.condition())
		for _, attribute := range []string{"temperature", "humidity", "wind_speed", "pressure", "current_temperature"} {
			if value, ok := number(weather.Attributes[attribute]); ok {
				for _, text := range numberVariants(value) {
					add(text, "weather."+entityObject(weather)+"."+attribute)
				}
			}
		}
		for _, index := range []int{0, 1} {
			if index >= len(weather.Daily) {
				break
			}
			f := weather.Daily[index]
			day := "today"
			if index == 1 {
				day = "tomorrow"
			}
			prefix := "weather." + entityObject(weather) + ".forecast." + day
			for _, phrase := range conditionVariants(f.Condition) {
				add(phrase, prefix+".condition")
			}
			for _, field := range []struct {
				name  string
				value *float64
			}{
				{"temperature", f.Temperature},
				{"templow", f.Templow},
				{"precipitation", f.Rain},
				{"precipitation_probability", f.Probability},
				{"wind_speed", f.WindSpeed},
			} {
				if field.value == nil {
					continue
				}
				for _, text := range numberVariants(*field.value) {
					add(text, prefix+"."+field.name)
				}
			}
		}
	}
	if q.Named != "" {
		if entity, ok := s.Entities[q.Named]; ok && entity.Domain != "weather" {
			add(stateWord(entity), entity.ID+".state")
		}
	} else if q.Kind == KindSensor || q.Kind == KindHumidity {
		// The caller sets Named to the sensor it found in the question.
		if entity, ok := s.Entities[q.Named]; ok && entity.Domain != "weather" {
			add(entity.State, entity.ID+".state")
		}
	}
	// Longest values first, so "19.5" is replaced before "19".
	sort.SliceStable(out, func(i, j int) bool {
		return len(out[i].text) > len(out[j].text)
	})
	// Drop duplicates with the same text: the first selector wins.
	seen := map[string]bool{}
	unique := out[:0]
	for _, c := range out {
		key := strings.ToLower(c.text)
		if seen[key] {
			continue
		}
		seen[key] = true
		unique = append(unique, c)
	}
	return unique
}

func conditionVariants(code string) []string {
	phrase := ConditionPhrase(code)
	switch phrase {
	case "":
		return nil
	case "lloviendo":
		return []string{"lloviendo", "llueve", "está lloviendo"}
	case "nevando":
		return []string{"nevando", "nieva", "está nevando"}
	case "soleado":
		return []string{"soleado", "sol", "hace sol"}
	}
	return []string{phrase}
}

func numberVariants(value float64) []string {
	plain := strconv.FormatFloat(value, 'f', -1, 64)
	if !strings.Contains(plain, ".") {
		return []string{plain}
	}
	return []string{plain, strings.ReplaceAll(plain, ".", ",")}
}

func entityObject(entity Entity) string {
	_, object, ok := strings.Cut(entity.ID, ".")
	if !ok {
		return entity.ID
	}
	return object
}

func resolveSelector(s Snapshot, selector string) (string, error) {
	parts := strings.SplitN(selector, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("%w: unknown placeholder %q", ErrBadTemplate, selector)
	}
	entityID := parts[0] + "." + parts[1]
	path := parts[2]
	entity, ok := s.Entities[entityID]
	if !ok {
		return "", fmt.Errorf("%w: unknown entity %q", ErrBadTemplate, entityID)
	}
	path = strings.TrimPrefix(path, "attributes.")
	switch {
	case path == "state":
		if entity.Unavailable() {
			return "", fmt.Errorf("%w: %s is unavailable", ErrBadTemplate, entityID)
		}
		return stateWord(entity), nil
	case path == "name":
		return entity.Name, nil
	case path == "unit":
		return entity.Unit, nil
	case path == "condition":
		phrase := ConditionPhrase(entity.condition())
		if phrase == "" {
			return "", fmt.Errorf("%w: %s has no known condition", ErrBadTemplate, entityID)
		}
		return phrase, nil
	}
	if strings.HasPrefix(path, "forecast.") {
		day, field, ok := strings.Cut(strings.TrimPrefix(path, "forecast."), ".")
		if !ok {
			return "", fmt.Errorf("%w: unknown placeholder %q", ErrBadTemplate, selector)
		}
		index, err := forecastIndex(day)
		if err != nil || index >= len(entity.Daily) {
			return "", fmt.Errorf("%w: %s has no forecast %s", ErrBadTemplate, entityID, day)
		}
		return forecastValue(entity.Daily[index], field)
	}
	value, ok := entity.Attributes[path]
	if !ok {
		return "", fmt.Errorf("%w: %s has no attribute %q", ErrBadTemplate, entityID, path)
	}
	if number, ok := number(value); ok {
		return formatNumber(number), nil
	}
	text := strings.TrimSpace(fmt.Sprint(value))
	if text == "" || text == "<nil>" {
		return "", fmt.Errorf("%w: %s has no value for %q", ErrBadTemplate, entityID, path)
	}
	return text, nil
}

func forecastIndex(day string) (int, error) {
	switch day {
	case "today":
		return 0, nil
	case "tomorrow":
		return 1, nil
	}
	index, err := strconv.Atoi(day)
	if err != nil || index < 0 || index > 7 {
		return 0, fmt.Errorf("bad forecast day %q", day)
	}
	return index, nil
}

func forecastValue(f Forecast, field string) (string, error) {
	switch field {
	case "condition":
		phrase := ConditionPhrase(f.Condition)
		if phrase == "" {
			return "", fmt.Errorf("%w: forecast has no condition", ErrBadTemplate)
		}
		return phrase, nil
	case "temperature":
		return optionalNumber(f.Temperature)
	case "templow":
		return optionalNumber(f.Templow)
	case "precipitation":
		return optionalNumber(f.Rain)
	case "precipitation_probability":
		return optionalNumber(f.Probability)
	case "wind_speed":
		return optionalNumber(f.WindSpeed)
	}
	return "", fmt.Errorf("%w: unknown forecast field %q", ErrBadTemplate, field)
}

func optionalNumber(value *float64) (string, error) {
	if value == nil {
		return "", fmt.Errorf("%w: forecast value is missing", ErrBadTemplate)
	}
	return formatNumber(*value), nil
}

func collapseSpaces(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
