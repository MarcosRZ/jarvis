package main

import (
	"strings"
	"testing"

	"jarvis/client/internal/commands"
)

func TestUserStepDone(t *testing.T) {
	tests := map[string]bool{
		`[{"step":"user","done":false},{"step":"core_config","done":false}]`: false,
		`[{"step":"user","done":true},{"step":"core_config","done":false}]`:  true,
	}
	for body, want := range tests {
		got, err := userStepDone(strings.NewReader(body))
		if err != nil || got != want {
			t.Errorf("%s: got %v, %v; want %v", body, got, err, want)
		}
	}
	if _, err := userStepDone(strings.NewReader(`[{"step":"core_config","done":true}]`)); err == nil {
		t.Error("no error for a status without the user step")
	}
}

func TestSpanishNamesAreValidDevices(t *testing.T) {
	devices := make([]commands.Device, 0, len(spanishNames))
	for id, name := range spanishNames {
		devices = append(devices, commands.Device{ID: id, Name: name})
	}
	if _, err := commands.New(nil, devices, nil); err != nil {
		t.Fatalf("the demo names are not a valid allowlist: %v", err)
	}
}
