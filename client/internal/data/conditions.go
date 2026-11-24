package data

import (
	"strings"
	"unicode"
)

// ConditionPhrase translates a Home Assistant weather condition into a phrase
// that fits after "está". Home Assistant uses English codes for every weather
// integration, so an untranslated code must never reach the speaker.
func ConditionPhrase(condition string) string {
	condition = strings.TrimSpace(condition)
	if phrase, ok := conditionPhrases[condition]; ok {
		return phrase
	}
	return ""
}

// conditionPhrases is also the allowlist of translated conditions. A template
// that contains one of these English codes as a literal word is rejected.
var conditionPhrases = map[string]string{
	"clear":           "despejado",
	"clear-night":     "despejado",
	"cloudy":          "nublado",
	"exceptional":     "con condiciones excepcionales",
	"fog":             "con niebla",
	"hail":            "granizando",
	"lightning":       "con tormenta",
	"lightning-rainy": "con tormenta y lluvia",
	"partlycloudy":    "parcialmente nublado",
	"pouring":         "lloviendo fuerte",
	"rainy":           "lloviendo",
	"snowy":           "nevando",
	"snowy-rainy":     "con aguanieve",
	"sunny":           "soleado",
	"windy":           "ventoso",
	"windy-variant":   "ventoso",
}

// englishConditions is the set of raw codes that a template may not contain.
var englishConditions = func() map[string]bool {
	set := make(map[string]bool, len(conditionPhrases))
	for code := range conditionPhrases {
		set[code] = true
	}
	return set
}()

var accents = strings.NewReplacer(
	"á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n",
)

// normalize lowercases text, removes Spanish accents and punctuation, and joins
// the words with single spaces.
func normalize(text string) string {
	text = accents.Replace(strings.ToLower(text))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, " ")
}
