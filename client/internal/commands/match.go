package commands

import (
	"cmp"
	"slices"
	"strings"
)

// Matching sets how Parse corrects misheard words when the exact match fails.
// A score is the similarity of how two phrases sound, from 0 to 1.
type Matching struct {
	// Accept is the lowest score of a guess that is used.
	Accept float64
	// Suggest is the lowest score of a guess that the reply suggests.
	Suggest float64
	// Margin is how much the best guess must beat the next one to be used.
	Margin float64
}

// DefaultMatching accepts "udelsalom" for "luz del salón" (0.82) and only
// suggests "luz de la cocina rm rf" for "luz de la cocina" (0.76). Suggest is
// above 0.69, the score of a wrong light that shares "luz del" with the right
// one, measured with the names of the development Home Assistant.
var DefaultMatching = Matching{Accept: 0.8, Suggest: 0.7, Margin: 0.1}

// Correction is one guess for a phrase that did not match exactly.
type Correction struct {
	Heard string
	Meant string
	Score float64
	Used  bool
}

type ranked[T any] struct {
	key   string
	value T
	score float64
}

// rank scores heard against each candidate key and returns the best key of each
// distinct value, best first, so that synonyms do not compete for the margin.
func rank[T comparable](heard string, candidates map[string]T) []ranked[T] {
	sound := phonetic(heard)
	best := map[T]ranked[T]{}
	for key, value := range candidates {
		r := ranked[T]{key: key, value: value, score: similarity(sound, phonetic(key))}
		if old, ok := best[value]; !ok || r.score > old.score || (r.score == old.score && r.key < old.key) {
			best[value] = r
		}
	}
	out := make([]ranked[T], 0, len(best))
	for _, r := range best {
		out = append(out, r)
	}
	slices.SortFunc(out, func(a, b ranked[T]) int {
		return cmp.Or(cmp.Compare(b.score, a.score), strings.Compare(a.key, b.key))
	})
	return out
}

// clear reports whether the best guess beats the next one by the margin.
func clear[T any](m Matching, r []ranked[T]) bool {
	return len(r) == 1 || r[0].score-r[1].score >= m.Margin
}

func (h *Handler) guessKind(heard string, candidates map[string]Kind) (Kind, *Correction) {
	r := rank(heard, candidates)
	if len(r) == 0 || r[0].score < h.matching.Accept || !clear(h.matching, r) {
		return "", nil
	}
	return r[0].value, &Correction{Heard: heard, Meant: r[0].key, Score: r[0].score, Used: true}
}

func (h *Handler) guessDevice(kind Kind, argument string, corrections []Correction) Command {
	r := rank(argument, h.devices)
	invalid := Command{Kind: KindInvalid, Argument: argument, Corrections: corrections}
	if argument == "" || len(r) == 0 || r[0].score < h.matching.Suggest {
		return invalid
	}
	unique := clear(h.matching, r)
	if unique && r[0].score >= h.matching.Accept {
		c := Correction{Heard: argument, Meant: r[0].value.Name, Score: r[0].score, Used: true}
		return Command{Kind: kind, Device: r[0].value, Corrections: append(corrections, c)}
	}
	for i, g := range r[:min(2, len(r))] {
		if i > 0 && (unique || g.score < h.matching.Suggest) {
			break
		}
		invalid.Suggestions = append(invalid.Suggestions, g.value)
		invalid.Corrections = append(invalid.Corrections, Correction{Heard: argument, Meant: g.value.Name, Score: g.score})
	}
	return invalid
}

// spanishSounds joins letters that sound the same in Spanish. "ch" becomes
// "C" so that the "c" rules do not split it; "gue" and "gui" keep a hard "g".
var spanishSounds = strings.NewReplacer(
	"ch", "C", "ll", "y", "qu", "k", "ce", "se", "ci", "si", "c", "k",
	"z", "s", "v", "b", "w", "b", "h", "", "gue", "ge", "gui", "gi", "ge", "je", "gi", "ji",
)

// phonetic returns how a normalized phrase sounds, without spaces, because
// Whisper often joins or splits words: "udelsalom" and "luz del salon" become
// "udelsalon" and "lusdelsalon".
func phonetic(phrase string) string {
	var b strings.Builder
	var last rune
	for _, word := range strings.Fields(phrase) {
		w := []rune(spanishSounds.Replace(word))
		if n := len(w); n > 1 {
			switch w[n-1] {
			case 'm':
				w[n-1] = 'n'
			case 'y':
				w[n-1] = 'i'
			}
		}
		for _, r := range w {
			if r != last {
				b.WriteRune(r)
			}
			last = r
		}
	}
	return b.String()
}

// similarity is 1 minus the edit distance divided by the longer length.
func similarity(a, b string) float64 {
	ra, rb := []rune(a), []rune(b)
	longer := max(len(ra), len(rb))
	if longer == 0 {
		return 1
	}
	return 1 - float64(levenshtein(ra, rb))/float64(longer)
}

func levenshtein(a, b []rune) int {
	prev := make([]int, len(b)+1)
	cur := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := range a {
		cur[0] = i + 1
		for j := range b {
			cost := 1
			if a[i] == b[j] {
				cost = 0
			}
			cur[j+1] = min(prev[j+1]+1, cur[j]+1, prev[j]+cost)
		}
		prev, cur = cur, prev
	}
	return prev[len(b)]
}
