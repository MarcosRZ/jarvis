package commands

import (
	"bytes"
	"context"
	"log/slog"
	"slices"
	"strings"
	"testing"
)

// devDevices are the names of the development Home Assistant.
var devDevices = []Device{
	{ID: "light.kitchen", Name: "La luz de la cocina"},
	{ID: "light.living_room", Name: "La luz del salón"},
	{ID: "light.bed_light", Name: "La luz del dormitorio"},
	{ID: "light.ceiling_lights", Name: "La luz del techo"},
	{ID: "light.office", Name: "La luz del despacho"},
	{ID: "light.entrance", Name: "La luz de la entrada"},
	{ID: "switch.decorative_lights", Name: "Las luces decorativas"},
	{ID: "switch.ac", Name: "El aire acondicionado"},
}

func devHandler(t *testing.T, controller DeviceController) *Handler {
	t.Helper()
	h, err := New(at(22, 15), devDevices, controller)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

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

func TestParseCorrectsMisheardDevices(t *testing.T) {
	h := devHandler(t, nil)
	tests := []struct {
		text   string
		kind   Kind
		device string
	}{
		{"Enciende el Udelsalom.", KindTurnOn, "light.living_room"},
		{"Enciende la luz del salom", KindTurnOn, "light.living_room"},
		{"Apaga la luz de salón", KindTurnOff, "light.living_room"},
		{"Apaga la luz de la cosina", KindTurnOff, "light.kitchen"},
		{"Enciende la luz de techo", KindTurnOn, "light.ceiling_lights"},
		{"Enciende luzdeltecho", KindTurnOn, "light.ceiling_lights"},
		{"Apaga el aire acondicionao", KindTurnOff, "switch.ac"},
		{"Enciendo la luz del despacho", KindTurnOn, "light.office"},
		{"Apague la luz del salón", KindTurnOff, "light.living_room"},
		{"¿Qué ora es?", KindTime, ""},
		{"Repítelo", KindRepeat, ""},
	}
	for _, tt := range tests {
		cmd := h.Parse(tt.text)
		if cmd.Kind != tt.kind || cmd.Device.ID != tt.device {
			t.Errorf("Parse(%q) = kind %q device %q, want %q %q (corrections %+v)", tt.text, cmd.Kind, cmd.Device.ID, tt.kind, tt.device, cmd.Corrections)
		}
	}
}

func TestParseOnlySuggestsUnclearDevices(t *testing.T) {
	h := devHandler(t, nil)
	tests := []struct {
		text        string
		suggestions []string
	}{
		{"Enciende la luz de la cocina; rm -rf /", []string{"light.kitchen"}},
		{"Enciende la luz del baño", []string{"light.living_room", "light.ceiling_lights"}},
		{"Enciende la luz de la cocina por favor", nil},
		{"Enciende el horno", nil},
		{"Enciende la luz", nil},
		{"Enciende el despacho", nil},
		{"Apaga la luz del dormitorio y la del salón", nil},
	}
	for _, tt := range tests {
		cmd := h.Parse(tt.text)
		var got []string
		for _, d := range cmd.Suggestions {
			got = append(got, d.ID)
		}
		if cmd.Kind != KindInvalid || !slices.Equal(got, tt.suggestions) {
			t.Errorf("Parse(%q) = kind %q suggestions %v, want invalid with %v", tt.text, cmd.Kind, got, tt.suggestions)
		}
	}
}

func TestSuggestionsAreInTheReplyAndNeverExecuted(t *testing.T) {
	devices := &fakeDevices{}
	h := devHandler(t, devices)
	tests := map[string]string{
		"Enciende la luz de la cocina; rm -rf /": `No conozco el dispositivo "luz de la cocina rm rf". ¿Querías decir la luz de la cocina?`,
		"Enciende la luz del baño":               `No conozco el dispositivo "luz del bano". ¿Querías decir la luz del salón o la luz del techo?`,
	}
	for text, want := range tests {
		if got := h.Handle(context.Background(), text); got != want {
			t.Errorf("Handle(%q) = %q, want %q", text, got, want)
		}
	}
	if len(devices.calls) != 0 {
		t.Fatalf("device calls %v for suggestions", devices.calls)
	}
}

func TestHandleLogsGuesses(t *testing.T) {
	var logs bytes.Buffer
	h := devHandler(t, &fakeDevices{})
	h.UseMatching(DefaultMatching, slog.New(slog.NewTextHandler(&logs, nil)), slog.LevelInfo)
	if got := h.Handle(context.Background(), "Enciende el Udelsalom."); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	want := `msg=guess heard=udelsalom meant="La luz del salón" score=0.82 used=true`
	if !strings.Contains(logs.String(), want) {
		t.Errorf("logs %q have no %q", logs.String(), want)
	}
}

func TestMatchingCanBeStricter(t *testing.T) {
	h := devHandler(t, &fakeDevices{})
	h.UseMatching(Matching{Accept: 1, Suggest: 0.8, Margin: 0.1}, nil, slog.LevelDebug)
	cmd := h.Parse("Enciende el Udelsalom.")
	if cmd.Kind != KindInvalid || len(cmd.Suggestions) != 1 || cmd.Suggestions[0].ID != "light.living_room" {
		t.Errorf("Parse = %+v, want only a suggestion with Accept 1", cmd)
	}
	if cmd := h.Parse("Enciende la luz del salom"); cmd.Kind != KindTurnOn {
		t.Errorf("a name that sounds the same is used also with Accept 1: %+v", cmd)
	}
}
