//go:build integration

package llm

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/commands"
)

// These tests need the llama.cpp service with the command model. They are
// skipped when VOICE_LLM_URL is not set.
func TestIntegrationInterpret(t *testing.T) {
	url := os.Getenv("VOICE_LLM_URL")
	if url == "" {
		t.Skip("VOICE_LLM_URL is not set")
	}
	resp, err := http.Get(strings.TrimRight(url, "/") + "/health")
	if err != nil || resp.StatusCode != http.StatusOK {
		if resp != nil {
			resp.Body.Close()
		}
		t.Skipf("llama.cpp is not ready: %v", err)
	}
	resp.Body.Close()
	devices := []commands.Device{
		{ID: "light.kitchen", Name: "la luz de la cocina"},
		{ID: "light.lounge", Name: "la luz del salón"},
		{ID: "switch.fan", Name: "el ventilador"},
	}
	c := New(Config{URL: url, Timeout: 3 * time.Minute, Room: "Salón"})
	tests := []struct {
		text string
		want []commands.Command
	}{
		{"enciende la luz del salón", []commands.Command{{Kind: commands.KindTurnOn, Device: devices[1]}}},
		{"apaga el ventilador", []commands.Command{{Kind: commands.KindTurnOff, Device: devices[2]}}},
		{"hay demasiada luz aquí", []commands.Command{{Kind: commands.KindTurnOff, Device: devices[1]}}},
		{"está muy oscuro", []commands.Command{{Kind: commands.KindTurnOn, Device: devices[1]}}},
		{"prende el ventilador y apaga la luz del salón", []commands.Command{
			{Kind: commands.KindTurnOn, Device: devices[2]},
			{Kind: commands.KindTurnOff, Device: devices[1]},
		}},
	}
	for _, tt := range tests {
		cmds, err := c.Interpret(context.Background(), tt.text, devices)
		if err != nil {
			t.Errorf("%q: %v", tt.text, err)
			continue
		}
		t.Logf("%q -> %+v", tt.text, cmds)
		if len(cmds) != len(tt.want) {
			t.Errorf("%q: %d commands, want %d", tt.text, len(cmds), len(tt.want))
			continue
		}
		for i := range cmds {
			if cmds[i].Kind != tt.want[i].Kind || cmds[i].Device.ID != tt.want[i].Device.ID {
				t.Errorf("%q: command %d is %+v, want %+v", tt.text, i, cmds[i], tt.want[i])
			}
		}
	}
}
