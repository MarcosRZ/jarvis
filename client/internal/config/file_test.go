package config

import (
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/commands"
)

func writeFile(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoadFileSetsBeep(t *testing.T) {
	path := writeFile(t, `
wake_word:
  beep:
    enabled: false
    notes: [440, 660.5, 880]
    note_duration: 50ms
    fade: 5ms
    level: -20
`)
	cfg, err := load([]string{"--config", path}, envFrom(nil), io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := audio.Beep{Notes: []float64{440, 660.5, 880}, NoteDuration: 50 * time.Millisecond, Fade: 5 * time.Millisecond, Level: -20}
	if cfg.BeepEnabled || !reflect.DeepEqual(cfg.Beep, want) {
		t.Errorf("enabled %v, beep %+v, want false and %+v", cfg.BeepEnabled, cfg.Beep, want)
	}
	if cfg.ConfigFile != path {
		t.Errorf("config file %q", cfg.ConfigFile)
	}
}

func TestLoadFileSetsMatching(t *testing.T) {
	path := writeFile(t, "commands:\n  matching:\n    accept: 0.9\n    margin: 0.2\n")
	cfg, err := load([]string{"--config", path}, envFrom(nil), io.Discard)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	want := commands.Matching{Accept: 0.9, Suggest: commands.DefaultMatching.Suggest, Margin: 0.2}
	if cfg.Matching != want {
		t.Errorf("matching %+v, want %+v", cfg.Matching, want)
	}
}

func TestLoadFileRejectsBadMatching(t *testing.T) {
	path := writeFile(t, "commands:\n  matching:\n    accept: 1.5\n    suggest: 0\n    margin: -0.1\n")
	_, err := load([]string{"--config", path}, envFrom(nil), io.Discard)
	for _, w := range []string{"commands.matching.accept 1.5", "commands.matching.suggest 0", "commands.matching.margin -0.1"} {
		if err == nil || !strings.Contains(err.Error(), w) {
			t.Errorf("error %v has no %q", err, w)
		}
	}
}

func TestLoadFileKeepsDefaultsForMissingKeys(t *testing.T) {
	for _, content := range []string{"", "wake_word:\n", "wake_word:\n  beep:\n    level: -30\n"} {
		cfg, err := load(nil, envFrom(map[string]string{EnvConfig: writeFile(t, content)}), io.Discard)
		if err != nil {
			t.Fatalf("%q: Load: %v", content, err)
		}
		want := audio.DefaultBeep
		if strings.Contains(content, "level") {
			want.Level = -30
		}
		if !cfg.BeepEnabled || !reflect.DeepEqual(cfg.Beep, want) {
			t.Errorf("%q: enabled %v, beep %+v", content, cfg.BeepEnabled, cfg.Beep)
		}
	}
}

func TestLoadWithoutFileDoesNotShareDefaultNotes(t *testing.T) {
	cfg, err := load(nil, envFrom(nil), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	cfg.Beep.Notes[0] = 1
	if audio.DefaultBeep.Notes[0] == 1 {
		t.Fatal("Config shares the notes slice with audio.DefaultBeep")
	}
}

func TestLoadFileRejectsInvalidValues(t *testing.T) {
	tests := []struct {
		name    string
		content string
		want    []string
	}{
		{"unknown key", "wake_word:\n  beep:\n    volume: 3\n", []string{"config.yaml: line 3: unknown key volume"}},
		{"wrong type", "wake_word:\n  beep:\n    notes: loud\n", []string{"config.yaml: line 3: cannot unmarshal"}},
		{"not yaml", "wake_word: [", []string{"config.yaml: yaml:"}},
		{"bad values", `
wake_word:
  beep:
    notes: []
    note_duration: 2s
    fade: soon
    level: 0
`, []string{
			"wake_word.beep.fade \"soon\": not a duration",
		}},
		{"bad ranges", `
wake_word:
  beep:
    notes: [10, 880]
    note_duration: 0s
    fade: -1ms
    level: -61
`, []string{
			"wake_word.beep.notes 10: use a frequency from 20 to 20000 Hz",
			"wake_word.beep.note_duration 0s: use a value greater than 0",
			"wake_word.beep.fade -1ms: use 0 to half of note_duration",
			"wake_word.beep.level -61: use a peak level from -60 to below 0 dBFS",
		}},
		{"no notes, full scale, long fade", `
wake_word:
  beep:
    notes: []
    fade: 50ms
    level: 0
`, []string{
			"wake_word.beep.notes []: use 1 to 8 frequencies",
			"wake_word.beep.fade 50ms: use 0 to half of note_duration (35ms)",
			"wake_word.beep.level 0:",
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := load([]string{"--config", writeFile(t, tt.content)}, envFrom(nil), io.Discard)
			if err == nil {
				t.Fatal("Load accepted the file")
			}
			for _, w := range tt.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q\nhas no %q", err, w)
				}
			}
		})
	}
}

func TestLoadReportsMissingFile(t *testing.T) {
	_, err := load([]string{"--config", "/nonexistent/config.yaml"}, envFrom(nil), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--config (VOICE_CONFIG)") {
		t.Fatalf("error %v", err)
	}
}
