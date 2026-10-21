package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"time"

	"go.yaml.in/yaml/v3"
)

const (
	maxBeepNotes        = 8
	minBeepNote         = 20.0
	maxBeepNote         = 20000.0
	maxBeepNoteDuration = time.Second
	minBeepLevel        = -60.0
)

var unknownField = regexp.MustCompile(`field (\S+) not found in type .*`)

// file is the YAML configuration file. Settings that are not in it keep their
// defaults; unknown keys are an error, so that a typo is not ignored.
type file struct {
	WakeWord struct {
		Beep struct {
			Enabled      *bool     `yaml:"enabled"`
			Notes        []float64 `yaml:"notes"`
			NoteDuration string    `yaml:"note_duration"`
			Fade         string    `yaml:"fade"`
			Level        *float64  `yaml:"level"`
		} `yaml:"beep"`
	} `yaml:"wake_word"`
	Commands struct {
		Matching struct {
			Accept  *float64 `yaml:"accept"`
			Suggest *float64 `yaml:"suggest"`
			Margin  *float64 `yaml:"margin"`
		} `yaml:"matching"`
	} `yaml:"commands"`
}

// loadFile applies the file at path to cfg. All invalid values are reported together.
func loadFile(path string, cfg *Config) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return fmt.Errorf("--%s (%s): %w", flagConfig, EnvConfig, err)
	}
	var f file
	dec := yaml.NewDecoder(bytes.NewReader(data))
	dec.KnownFields(true)
	if err := dec.Decode(&f); err != nil && !errors.Is(err, io.EOF) {
		var typeErr *yaml.TypeError
		if !errors.As(err, &typeErr) {
			return fmt.Errorf("%s: %w", path, err)
		}
		errs := make([]error, 0, len(typeErr.Errors))
		for _, e := range typeErr.Errors {
			// "line 3: field levle not found in type struct {...}" names a Go type.
			e = unknownField.ReplaceAllString(e, "unknown key $1")
			errs = append(errs, fmt.Errorf("%s: %s", path, e))
		}
		return errors.Join(errs...)
	}

	var errs []error
	bad := func(key string, value any, rule string) {
		errs = append(errs, fmt.Errorf("%s: wake_word.beep.%s %v: %s", path, key, value, rule))
	}
	badMatching := func(key string, value any, rule string) {
		errs = append(errs, fmt.Errorf("%s: commands.matching.%s %v: %s", path, key, value, rule))
	}
	m := f.Commands.Matching
	for _, v := range []struct {
		value  *float64
		target *float64
	}{{m.Accept, &cfg.Matching.Accept}, {m.Suggest, &cfg.Matching.Suggest}, {m.Margin, &cfg.Matching.Margin}} {
		if v.value != nil {
			*v.target = *v.value
		}
	}
	match := cfg.Matching
	if !(match.Accept > 0 && match.Accept <= 1) {
		badMatching("accept", match.Accept, "use a score greater than 0 and at most 1; 1 accepts only names that sound the same")
	}
	if !(match.Suggest > 0 && match.Suggest <= match.Accept) {
		badMatching("suggest", match.Suggest, fmt.Sprintf("use a score greater than 0 and at most accept (%v)", match.Accept))
	}
	if !(match.Margin >= 0 && match.Margin < 1) {
		badMatching("margin", match.Margin, "use a value from 0 to below 1")
	}
	b := f.WakeWord.Beep
	if b.Enabled != nil {
		cfg.BeepEnabled = *b.Enabled
	}
	if b.Notes != nil {
		cfg.Beep.Notes = b.Notes
	}
	for key, d := range map[string]struct {
		text   string
		target *time.Duration
	}{
		"note_duration": {b.NoteDuration, &cfg.Beep.NoteDuration},
		"fade":          {b.Fade, &cfg.Beep.Fade},
	} {
		if d.text == "" {
			continue
		}
		parsed, err := time.ParseDuration(d.text)
		if err != nil {
			bad(key, fmt.Sprintf("%q", d.text), "not a duration; use a value such as 70ms")
			continue
		}
		*d.target = parsed
	}
	if b.Level != nil {
		cfg.Beep.Level = *b.Level
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	beep := cfg.Beep
	if len(beep.Notes) == 0 || len(beep.Notes) > maxBeepNotes {
		bad("notes", beep.Notes, fmt.Sprintf("use 1 to %d frequencies in Hz", maxBeepNotes))
	}
	for _, n := range beep.Notes {
		if !(n >= minBeepNote && n <= maxBeepNote) {
			bad("notes", n, fmt.Sprintf("use a frequency from %v to %v Hz", minBeepNote, maxBeepNote))
		}
	}
	if beep.NoteDuration <= 0 || beep.NoteDuration > maxBeepNoteDuration {
		bad("note_duration", beep.NoteDuration, fmt.Sprintf("use a value greater than 0 and at most %v", maxBeepNoteDuration))
	}
	if beep.Fade < 0 || beep.Fade > beep.NoteDuration/2 {
		bad("fade", beep.Fade, fmt.Sprintf("use 0 to half of note_duration (%v)", beep.NoteDuration/2))
	}
	if !(beep.Level >= minBeepLevel && beep.Level < 0) {
		bad("level", beep.Level, fmt.Sprintf("use a peak level from %v to below 0 dBFS", minBeepLevel))
	}
	return errors.Join(errs...)
}
