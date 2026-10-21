//go:build integration

package homeassistant

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"testing"
	"time"

	"jarvis/client/internal/commands"
)

type Device = commands.Device

// These tests need a Home Assistant with the demo integration, set up by
// "make ha-up". They are skipped when VOICE_HA_URL or VOICE_HA_TOKEN is not set.

func integrationClient(t *testing.T) *Client {
	t.Helper()
	url, token := os.Getenv("VOICE_HA_URL"), os.Getenv("VOICE_HA_TOKEN")
	if url == "" || token == "" {
		t.Skip("VOICE_HA_URL and VOICE_HA_TOKEN are not set")
	}
	return New(Config{URL: url, Token: token, Timeout: 10 * time.Second})
}

func lightState(t *testing.T, c *Client, id string) string {
	t.Helper()
	var s struct {
		State string `json:"state"`
	}
	if err := c.rest(context.Background(), http.MethodGet, "/api/states/"+id, nil, &s); err != nil {
		t.Fatalf("state of %s: %v", id, err)
	}
	return s.State
}

func TestIntegrationDevices(t *testing.T) {
	c := integrationClient(t)
	devices, err := c.Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	b, _ := json.Marshal(devices)
	t.Logf("exposed devices: %s", b)
	if len(devices) == 0 {
		t.Fatal("no exposed lights or switches")
	}
}

func setExposed(t *testing.T, c *Client, id string, exposed bool) {
	t.Helper()
	s, err := Connect(context.Background(), c.cfg)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	command := map[string]any{"type": "homeassistant/expose_entity", "assistants": []string{assistant}, "entity_ids": []string{id}, "should_expose": exposed}
	if err := s.Call(context.Background(), command, nil); err != nil {
		t.Fatal(err)
	}
}

func contains(devices []Device, id string) bool {
	for _, d := range devices {
		if d.ID == id {
			return true
		}
	}
	return false
}

func TestIntegrationExposureChangeNeedsNoRestart(t *testing.T) {
	c := integrationClient(t)
	devices, err := c.Devices(context.Background())
	if err != nil || len(devices) == 0 {
		t.Fatalf("Devices: %v, %d devices", err, len(devices))
	}
	id := devices[0].ID
	setExposed(t, c, id, false)
	t.Cleanup(func() { setExposed(t, c, id, true) })

	if devices, _ = c.Devices(context.Background()); contains(devices, id) {
		t.Fatalf("%s is still listed after it was un-exposed", id)
	}
	setExposed(t, c, id, true)
	if devices, _ = c.Devices(context.Background()); !contains(devices, id) {
		t.Fatalf("%s is not listed after it was exposed again", id)
	}
}

func TestIntegrationSetPower(t *testing.T) {
	c := integrationClient(t)
	devices, err := c.Devices(context.Background())
	if err != nil || len(devices) == 0 {
		t.Fatalf("Devices: %v, %d devices", err, len(devices))
	}
	id := devices[0].ID
	for _, on := range []bool{false, true, false} {
		if err := c.SetPower(context.Background(), id, on); err != nil {
			t.Fatalf("SetPower(%s, %v): %v", id, on, err)
		}
		want := "off"
		if on {
			want = "on"
		}
		if got := lightState(t, c, id); got != want {
			t.Errorf("%s is %q after SetPower(%v), want %q", id, got, on, want)
		}
	}
}
