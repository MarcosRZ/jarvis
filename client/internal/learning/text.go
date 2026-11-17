package learning

import (
	"strings"
	"unicode"
)

var plainLetters = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// Normalize lowercases text, removes Spanish accents and punctuation, and
// joins the words with single spaces. It is the same normalization the
// command handler applies before it stores or looks up a learned sentence.
func Normalize(text string) string {
	text = plainLetters.Replace(strings.ToLower(text))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, " ")
}

// fillers are words that do not change the command. They are common at the end
// of a spoken sentence ("apaga el ventilador, por favor") and would otherwise
// make the device phrase unknown.
var fillers = map[string]bool{"por": true, "favor": true, "porfa": true, "please": true, "gracias": true}

// WithoutFillers removes the filler words from a normalized sentence.
func WithoutFillers(norm string) string {
	words := strings.Fields(norm)
	kept := words[:0]
	for _, word := range words {
		if !fillers[word] {
			kept = append(kept, word)
		}
	}
	return strings.Join(kept, " ")
}

// Key is the normalized sentence without fillers, the key that Store uses.
func Key(text string) string {
	return WithoutFillers(Normalize(text))
}
