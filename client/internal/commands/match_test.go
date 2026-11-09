package commands

import (
	"bytes"
	"log/slog"
	"strings"
	"testing"
)

func TestPhonetic(t *testing.T) {
	tests := map[string]string{
		"udelsalom":         "udelsalon",
		"luz del salon":     "lusdelsalon",
		"que hora es":       "keoraes",
		"techo":             "teCo",
		"llave":             "yabe",
		"cocina":            "kosina",
		"gente":             "jente",
		"perro hoy":         "peroi",
		"aire acondicionao": "aireakondisionao",
		"apague":            "apage",
		"guitarra":          "gitara",
	}
	for in, want := range tests {
		if got := phonetic(in); got != want {
			t.Errorf("phonetic(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseCorrectsMisheardFixedCommands(t *testing.T) {
	h := newHandler(t, nil)
	tests := []struct {
		text string
		kind Kind
	}{
		{"¿Qué ora es?", KindTime},
		{"Repítelo", KindRepeat},
	}
	for _, tt := range tests {
		cmd := h.Parse(tt.text)
		if cmd.Kind != tt.kind {
			t.Errorf("Parse(%q) = kind %q, want %q (corrections %+v)", tt.text, cmd.Kind, tt.kind, cmd.Corrections)
		}
	}
}

func TestHandleLogsGuesses(t *testing.T) {
	var logs bytes.Buffer
	h := newHandler(t, nil)
	h.UseMatching(DefaultMatching, slog.New(slog.NewTextHandler(&logs, nil)), slog.LevelInfo)
	if got := h.Handle(t.Context(), "¿Qué ora es?"); got == ReplyUnknown {
		t.Fatalf("reply %q", got)
	}
	want := `msg=guess heard="que ora es" meant="que hora es"`
	if !strings.Contains(logs.String(), want) {
		t.Errorf("logs %q have no %q", logs.String(), want)
	}
}

func TestMatchingCanBeStricter(t *testing.T) {
	h := newHandler(t, nil)
	h.UseMatching(Matching{Accept: 1.01, Margin: 0.1}, nil, slog.LevelDebug)
	if cmd := h.Parse("¿Qué ora es?"); cmd.Kind != KindUnknown {
		t.Errorf("Parse = %+v, want unknown with Accept 1", cmd)
	}
	if cmd := h.Parse("¿Qué hora es?"); cmd.Kind != KindTime {
		t.Errorf("exact phrase must still match with Accept 1: %+v", cmd)
	}
}
