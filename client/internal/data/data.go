// Package data answers questions about the home from a read-only snapshot of
// the entities exposed in Home Assistant, and from the news service. It is the
// deterministic L1 layer: the language model is only used when this package has
// no answer.
//
// Answers are templates. A built-in template is correct by construction; a
// template taught by the user is validated and rendered with the snapshot, so a
// changing value is never frozen in memory.
package data

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Entity is one exposed Home Assistant entity in a snapshot.
type Entity struct {
	ID         string
	Name       string
	Domain     string
	State      string
	Unit       string
	Attributes map[string]any
	Daily      []Forecast
	Hourly     []Forecast
}

// Forecast is one entry of a weather forecast.
type Forecast struct {
	Time        time.Time
	Condition   string
	Temperature *float64
	Templow     *float64
	Rain        *float64
	Probability *float64
	WindSpeed   *float64
}

// Snapshot is the read-only state used to answer one question. Weather lists
// the weather entities in preference order, for example weather.aemet first.
type Snapshot struct {
	Entities map[string]Entity
	Weather  []string
	At       time.Time
}

// Provider returns a snapshot of the exposed entities.
type Provider interface {
	DataSnapshot(ctx context.Context) (Snapshot, error)
}

// News returns the headlines to read, already formatted as "Source: title.".
// An error means that the news service is unreachable.
type News interface {
	Headlines(ctx context.Context) ([]string, error)
}

// Kind is the type of a data question.
type Kind int

const (
	KindNone Kind = iota
	KindTemperature
	KindWeather
	KindRain
	KindUmbrella
	KindHumidity
	KindEntityState
	KindNews
	KindDate
	KindSensor
)

// Question is one detected data question. Day is 0 for today, 1 for tomorrow
// and N for "in N days"; Weekday is 0 for Sunday and -1 when the sentence names
// no weekday. Named is the entity of an entity-state question, set by the
// caller when it can resolve it.
type Question struct {
	Kind    Kind
	Day     int
	Weekday int
	Named   string
}

// IsData reports whether the question has a data kind.
func (q Question) IsData() bool {
	return q.Kind != KindNone
}

// Result is the outcome of Resolve.
type Result int

const (
	// NoMatch means that the question is not one this package can answer, so
	// the caller must use its fallback.
	NoMatch Result = iota
	// Answered means that Text is the spoken answer.
	Answered
	// Missing means that the question is a data question but the value is not
	// available. The answer is the data-missing failure phrase.
	Missing
	// NewsDown means that the news service is unreachable.
	NewsDown
)

// Resolver answers data questions. Now and Location are optional; the zero
// value uses time.Now and the local time zone.
type Resolver struct {
	Now      func() time.Time
	Location *time.Location
}

// Resolve answers q from the snapshot. headlines and newsErr are the result of
// the news lookup, used only by KindNews.
func (r Resolver) Resolve(s Snapshot, q Question, headlines []string, newsErr error) (string, Result) {
	switch q.Kind {
	case KindTemperature:
		return r.temperature(s, q)
	case KindWeather:
		return r.weather(s, q)
	case KindHumidity:
		return r.humidity(s)
	case KindRain:
		return r.rain(s, q)
	case KindUmbrella:
		return r.umbrella(s, q)
	case KindEntityState:
		return r.entityState(s, q.Named)
	case KindSensor:
		return r.sensor(s, q)
	case KindNews:
		if newsErr != nil {
			return "", NewsDown
		}
		if len(headlines) == 0 {
			return "", Missing
		}
		return strings.Join(headlines, " "), Answered
	}
	return "", NoMatch
}

func (r Resolver) now(s Snapshot) time.Time {
	if r.Now != nil {
		return r.Now()
	}
	if !s.At.IsZero() {
		return s.At
	}
	return time.Now()
}

func (r Resolver) location() *time.Location {
	if r.Location != nil {
		return r.Location
	}
	return time.Local
}

// WeatherEntity returns the preferred weather entity of the snapshot.
func (s Snapshot) WeatherEntity() (Entity, bool) {
	for _, id := range s.Weather {
		if e, ok := s.Entities[id]; ok && e.Domain == "weather" {
			return e, true
		}
	}
	ids := make([]string, 0)
	for id, e := range s.Entities {
		if e.Domain == "weather" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return Entity{}, false
	}
	return s.Entities[ids[0]], true
}

func (r Resolver) temperature(s Snapshot, q Question) (string, Result) {
	w, ok := s.WeatherEntity()
	if !ok {
		return "", Missing
	}
	var t float64
	var conditional string
	if q.Day > 0 || q.Weekday >= 0 {
		forecast, ok := r.forecastFor(w, q)
		if !ok || forecast.Temperature == nil {
			return "", Missing
		}
		t = *forecast.Temperature
		conditional = ConditionPhrase(forecast.Condition)
	} else {
		value, ok := number(w.Attributes["temperature"])
		if !ok {
			return "", Missing
		}
		t = value
		conditional = ConditionPhrase(w.condition())
	}
	if conditional == "" {
		return fmt.Sprintf("Hay %s grados.", formatNumber(t)), Answered
	}
	return fmt.Sprintf("Hay %s grados y está %s.", formatNumber(t), conditional), Answered
}

func (r Resolver) weather(s Snapshot, q Question) (string, Result) {
	w, ok := s.WeatherEntity()
	if !ok {
		return "", Missing
	}
	condition := ""
	var temperature *float64
	if q.Day > 0 || q.Weekday >= 0 {
		forecast, ok := r.forecastFor(w, q)
		if !ok {
			return "", Missing
		}
		condition = ConditionPhrase(forecast.Condition)
		temperature = forecast.Temperature
	} else {
		condition = ConditionPhrase(w.condition())
		if value, ok := number(w.Attributes["temperature"]); ok {
			temperature = &value
		}
	}
	switch {
	case condition != "" && temperature != nil:
		return fmt.Sprintf("Está %s y hay %s grados.", condition, formatNumber(*temperature)), Answered
	case condition != "":
		return fmt.Sprintf("Está %s.", condition), Answered
	case temperature != nil:
		return fmt.Sprintf("Hay %s grados.", formatNumber(*temperature)), Answered
	}
	return "", Missing
}

func (r Resolver) humidity(s Snapshot) (string, Result) {
	if w, ok := s.WeatherEntity(); ok {
		if value, ok := number(w.Attributes["humidity"]); ok {
			return fmt.Sprintf("Hay un %s por ciento de humedad.", formatNumber(value)), Answered
		}
	}
	entity, ok := s.FindSensor("humedad")
	if !ok {
		return "", Missing
	}
	value, ok := numberText(entity.State)
	if !ok {
		return "", Missing
	}
	if entity.Unit == "%" {
		return fmt.Sprintf("Hay un %s por ciento de humedad.", formatNumber(value)), Answered
	}
	if entity.Unit != "" {
		return fmt.Sprintf("La humedad es %s %s.", formatNumber(value), entity.Unit), Answered
	}
	return fmt.Sprintf("La humedad es %s.", formatNumber(value)), Answered
}

func (r Resolver) rain(s Snapshot, q Question) (string, Result) {
	w, ok := s.WeatherEntity()
	if !ok {
		return "", Missing
	}
	if q.Day == 0 && q.Weekday < 0 {
		switch conditionRain(w.condition()) {
		case rainYes:
			return "Sí, está lloviendo.", Answered
		case rainNo:
			return "No, no está lloviendo.", Answered
		}
		return "", Missing
	}
	forecast, ok := r.forecastFor(w, q)
	if !ok {
		return "", Missing
	}
	probability, hasProbability := forecastProbability(forecast)
	condition := ConditionPhrase(forecast.Condition)
	willRain := conditionRain(forecast.Condition) == rainYes
	if hasProbability {
		if probability >= 50 || willRain {
			return fmt.Sprintf("Sí, va a llover. Hay un %s por ciento de probabilidad.", formatNumber(probability)), Answered
		}
		return fmt.Sprintf("No, no va a llover. Hay un %s por ciento de probabilidad.", formatNumber(probability)), Answered
	}
	if willRain {
		return "Sí, va a llover.", Answered
	}
	if condition != "" {
		return "No, no va a llover.", Answered
	}
	return "", Missing
}

func (r Resolver) umbrella(s Snapshot, q Question) (string, Result) {
	w, ok := s.WeatherEntity()
	if !ok {
		return "", Missing
	}
	probability, ok := r.rainProbability(w, q)
	if !ok {
		return "", Missing
	}
	if probability >= 50 {
		return fmt.Sprintf("Hay un %s por ciento de probabilidad de lluvia. Lleva paraguas.", formatNumber(probability)), Answered
	}
	return fmt.Sprintf("Hay un %s por ciento de probabilidad de lluvia. No hace falta paraguas.", formatNumber(probability)), Answered
}

func (r Resolver) rainProbability(w Entity, q Question) (float64, bool) {
	if q.Day == 0 && q.Weekday < 0 {
		// Look at the next hours first: "now" is better described by the hourly
		// forecast than by the daily one.
		max := 0.0
		found := false
		limit := r.now(Snapshot{}).Add(12 * time.Hour)
		for _, f := range w.Hourly {
			if !f.Time.IsZero() && f.Time.After(limit) {
				continue
			}
			if f.Probability != nil {
				found = true
				max = maxFloat(max, *f.Probability)
			}
			if f.Condition != "" && conditionRain(f.Condition) == rainYes {
				found = true
				max = maxFloat(max, 100)
			}
		}
		if found {
			return max, true
		}
	}
	f, ok := r.forecastFor(w, q)
	if !ok {
		return 0, false
	}
	return forecastProbability(f)
}

func (r Resolver) entityState(s Snapshot, id string) (string, Result) {
	entity, ok := s.Entities[id]
	if !ok || entity.Unavailable() {
		return "", Missing
	}
	switch entity.State {
	case "on":
		return "Sí, está " + stateAdjective(entity.Name, "encendido") + ".", Answered
	case "off":
		return "No, está " + stateAdjective(entity.Name, "apagado") + ".", Answered
	case "open":
		return "Sí, está " + stateAdjective(entity.Name, "abierto") + ".", Answered
	case "closed":
		return "No, está " + stateAdjective(entity.Name, "cerrado") + ".", Answered
	case "home":
		return "Sí, está en casa.", Answered
	case "not_home":
		return "No, no está en casa.", Answered
	}
	return "", Missing
}

// stateWord is the Spanish word of an on/off or open/closed state, with the
// gender of the device name. Sensors return their raw value.
func stateWord(entity Entity) string {
	switch entity.State {
	case "on":
		return stateAdjective(entity.Name, "encendido")
	case "off":
		return stateAdjective(entity.Name, "apagado")
	case "open":
		return stateAdjective(entity.Name, "abierto")
	case "closed":
		return stateAdjective(entity.Name, "cerrado")
	case "home":
		return "en casa"
	case "not_home":
		return "fuera de casa"
	}
	return strings.TrimSpace(entity.State)
}

func (r Resolver) sensor(s Snapshot, q Question) (string, Result) {
	// The sensor name is already in the question; FindSensor uses it.
	if q.Named != "" {
		if entity, ok := s.Entities[q.Named]; ok {
			return sensorAnswer(entity)
		}
	}
	return "", NoMatch
}

// sensorAnswer answers one sensor with a sentence that agrees with its unit.
func sensorAnswer(entity Entity) (string, Result) {
	if entity.Unavailable() {
		return "", Missing
	}
	value := strings.TrimSpace(entity.State)
	if value == "" {
		return "", Missing
	}
	name := strings.ToLower(entity.Name)
	switch {
	case entity.Unit == "%" && strings.Contains(name, "humedad"):
		return fmt.Sprintf("Hay un %s por ciento de humedad.", value), Answered
	case entity.Unit == "%" && strings.Contains(name, "bateria"):
		return fmt.Sprintf("La batería está al %s por ciento.", value), Answered
	case entity.Unit == "°C" && strings.Contains(name, "temperatura"):
		return fmt.Sprintf("La temperatura es de %s grados.", value), Answered
	case entity.Unit != "":
		return fmt.Sprintf("El valor de %s es %s %s.", name, value, entity.Unit), Answered
	}
	return fmt.Sprintf("El valor de %s es %s.", name, value), Answered
}

// FindSensor returns the exposed sensor whose name the text names most clearly.
// It ignores weather entities, which are covered by their own questions.
func (s Snapshot) FindSensor(text string) (Entity, bool) {
	words := significantWords(text)
	if len(words) == 0 {
		return Entity{}, false
	}
	type scored struct {
		entity Entity
		score  float64
	}
	candidates := make([]scored, 0, len(s.Entities))
	for _, entity := range s.Entities {
		if entity.Domain == "weather" || entity.Unavailable() {
			continue
		}
		names := significantWords(entity.Name)
		if len(names) == 0 {
			continue
		}
		matches := 0
		for _, name := range names {
			for _, word := range words {
				if sameWord(word, name) {
					matches++
					break
				}
			}
		}
		score := float64(matches) / float64(len(names))
		if score >= 0.5 {
			candidates = append(candidates, scored{entity: entity, score: score})
		}
	}
	if len(candidates) == 0 {
		return Entity{}, false
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].entity.ID < candidates[j].entity.ID
	})
	if len(candidates) > 1 && candidates[0].score-candidates[1].score < 0.25 {
		return Entity{}, false
	}
	return candidates[0].entity, true
}

// Unavailable reports whether the entity has no usable state.
func (e Entity) Unavailable() bool {
	switch strings.ToLower(strings.TrimSpace(e.State)) {
	case "", "unavailable", "unknown", "none", "<nil>":
		return true
	}
	return false
}

func (e Entity) condition() string {
	if value, ok := e.Attributes["condition"]; ok {
		if text := strings.TrimSpace(fmt.Sprint(value)); text != "" && text != "<nil>" {
			return text
		}
	}
	return e.State
}

type rainKind int

const (
	rainUnknown rainKind = iota
	rainYes
	rainNo
)

func conditionRain(condition string) rainKind {
	switch strings.TrimSpace(condition) {
	case "rainy", "pouring", "lightning-rainy", "hail", "snowy-rainy":
		return rainYes
	case "clear", "clear-night", "sunny", "cloudy", "partlycloudy", "windy", "windy-variant", "fog", "snowy", "exceptional":
		return rainNo
	}
	return rainUnknown
}

func (r Resolver) forecastFor(w Entity, q Question) (Forecast, bool) {
	target := r.now(Snapshot{})
	if q.Weekday >= 0 {
		days := (q.Weekday - int(target.Weekday()) + 7) % 7
		if days == 0 {
			days = 7
		}
		target = target.AddDate(0, 0, days)
	} else if q.Day > 0 {
		target = target.AddDate(0, 0, q.Day)
	}
	location := r.location()
	return findForecast(w.Daily, target, location)
}

func findForecast(list []Forecast, target time.Time, location *time.Location) (Forecast, bool) {
	for _, f := range list {
		if f.Time.IsZero() {
			continue
		}
		y, m, d := f.Time.In(location).Date()
		ty, tm, td := target.In(location).Date()
		if y == ty && m == tm && d == td {
			return f, true
		}
	}
	return Forecast{}, false
}

func forecastProbability(f Forecast) (float64, bool) {
	if f.Probability != nil {
		return *f.Probability, true
	}
	switch conditionRain(f.Condition) {
	case rainYes:
		return 100, true
	case rainNo:
		return 0, true
	}
	return 0, false
}

// Temperature returns the current temperature sentence, for a physical state
// that no configured device can fix. The second value is false when no weather
// entity has a temperature.
func (s Snapshot) Temperature(now time.Time) (string, bool) {
	r := Resolver{Now: func() time.Time { return now }}
	text, result := r.temperature(s, Question{Weekday: -1})
	return text, result == Answered
}

// Humidity returns the current humidity sentence, for a physical state that no
// configured device can fix.
func (s Snapshot) Humidity() (string, bool) {
	r := Resolver{}
	text, result := r.humidity(s)
	return text, result == Answered
}

func stateAdjective(name, base string) string {
	if feminine(name) {
		return strings.TrimSuffix(base, "o") + "a"
	}
	return base
}

// feminine guesses the gender of a device name so that "encendida" agrees with
// "la luz" and "encendido" with "el enchufe". It is a heuristic: the article of
// the name wins, then a list of common devices, then the final letter.
func feminine(name string) bool {
	words := strings.Fields(strings.ToLower(name))
	if len(words) == 0 {
		return false
	}
	switch words[0] {
	case "la", "las", "una":
		return true
	case "el", "los", "un":
		return false
	}
	irregular := map[string]bool{
		"luz": true, "tele": true, "television": true, "lampara": true,
		"calefaccion": true, "persiana": true, "puerta": true, "ventana": true,
		"lavadora": true, "nevera": true, "cocina": true, "bombilla": true,
		"estufa": true, "rutina": true, "escena": true,
	}
	word := words[0]
	if irregular[word] {
		return true
	}
	return strings.HasSuffix(word, "a")
}

func number(value any) (float64, bool) {
	switch v := value.(type) {
	case float64:
		return v, true
	case float32:
		return float64(v), true
	case int:
		return float64(v), true
	case int64:
		return float64(v), true
	case jsonNumber:
		f, err := strconv.ParseFloat(string(v), 64)
		return f, err == nil
	}
	return numberText(fmt.Sprint(value))
}

type jsonNumber string

func numberText(text string) (float64, bool) {
	text = strings.TrimSpace(strings.ReplaceAll(text, ",", "."))
	f, err := strconv.ParseFloat(text, 64)
	return f, err == nil
}

func formatNumber(value float64) string {
	return strconv.FormatFloat(value, 'f', -1, 64)
}

func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}

func significantWords(text string) []string {
	words := strings.Fields(normalize(text))
	kept := make([]string, 0, len(words))
	for _, word := range words {
		if len([]rune(word)) < 3 || stopWords[word] {
			continue
		}
		kept = append(kept, word)
	}
	return kept
}

func sameWord(a, b string) bool {
	return a == b || a == b+"s" || b == a+"s"
}

var stopWords = map[string]bool{
	"aqui": true, "ahora": true, "cual": true, "cuando": true, "cuanta": true,
	"cuantas": true, "cuanto": true, "cuantos": true, "del": true, "dime": true,
	"esta": true, "estan": true, "favor": true, "hay": true, "hace": true,
	"para": true, "por": true, "que": true, "tiene": true, "valor": true,
}
