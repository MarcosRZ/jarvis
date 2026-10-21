// Package homeassistant reads the devices exposed to voice assistants in Home
// Assistant and switches them on and off. It implements commands.DeviceController.
package homeassistant

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"slices"
	"strings"
	"time"

	"jarvis/client/internal/commands"
)

// assistant is the key of the Home Assistant voice assistant in the exposure settings.
const assistant = "conversation"

const maxResponseBytes = 8 << 20

// powerDomains are the entity domains that the assistant may switch.
var powerDomains = map[string]bool{"light": true, "switch": true}

// entityID is the Home Assistant entity ID form: domain.object_id, lowercase.
var entityID = regexp.MustCompile(`^([a-z0-9_]+)\.([a-z0-9_]+)$`)

var (
	ErrUnauthorized = errors.New("Home Assistant rejected the access token")
	ErrNotAllowed   = errors.New("entity is not a light or a switch")
)

type Config struct {
	// URL is the Home Assistant base URL, for example http://127.0.0.1:8123.
	URL     string
	Token   string
	Timeout time.Duration
}

type Client struct {
	cfg  Config
	http *http.Client
}

func New(cfg Config) *Client {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	return &Client{cfg: cfg, http: &http.Client{Timeout: cfg.Timeout}}
}

// Devices returns the lights and switches exposed to the Home Assistant voice
// assistant, sorted by name. The name is the friendly name, or the object ID
// with spaces when there is none. A device in the state "unavailable" is in
// the list, marked Unavailable.
func (c *Client) Devices(ctx context.Context) ([]commands.Device, error) {
	exposed, err := c.exposedEntities(ctx)
	if err != nil {
		return nil, err
	}
	var states []struct {
		EntityID   string `json:"entity_id"`
		State      string `json:"state"`
		Attributes struct {
			FriendlyName string `json:"friendly_name"`
		} `json:"attributes"`
	}
	if err := c.rest(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return nil, err
	}
	var devices []commands.Device
	for _, s := range states {
		m := entityID.FindStringSubmatch(s.EntityID)
		if m == nil || !powerDomains[m[1]] || !exposed[s.EntityID][assistant] {
			continue
		}
		name := strings.TrimSpace(s.Attributes.FriendlyName)
		if name == "" {
			name = strings.ReplaceAll(m[2], "_", " ")
		}
		devices = append(devices, commands.Device{ID: s.EntityID, Name: name, Unavailable: s.State == "unavailable"})
	}
	slices.SortFunc(devices, func(a, b commands.Device) int { return strings.Compare(a.Name, b.Name) })
	return devices, nil
}

// SetPower calls the turn_on or turn_off service of the entity. It accepts
// only light and switch entity IDs.
func (c *Client) SetPower(ctx context.Context, id string, on bool) error {
	m := entityID.FindStringSubmatch(id)
	if m == nil || !powerDomains[m[1]] {
		return fmt.Errorf("%w: %q", ErrNotAllowed, id)
	}
	service := "turn_off"
	if on {
		service = "turn_on"
	}
	body := map[string]string{"entity_id": id}
	return c.rest(ctx, http.MethodPost, "/api/services/"+m[1]+"/"+service, body, nil)
}

func (c *Client) exposedEntities(ctx context.Context) (map[string]map[string]bool, error) {
	s, err := Connect(ctx, c.cfg)
	if err != nil {
		return nil, err
	}
	defer s.Close()
	var result struct {
		ExposedEntities map[string]map[string]bool `json:"exposed_entities"`
	}
	if err := s.Call(ctx, map[string]any{"type": "homeassistant/expose_entity/list"}, &result); err != nil {
		return nil, err
	}
	return result.ExposedEntities, nil
}

func (c *Client) rest(ctx context.Context, method, path string, body, out any) error {
	var reader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		reader = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.cfg.URL+path, reader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.cfg.Token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("Home Assistant %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode != http.StatusOK:
		return fmt.Errorf("Home Assistant %s %s: HTTP %d", method, path, resp.StatusCode)
	case out == nil:
		return nil
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(out); err != nil {
		return fmt.Errorf("Home Assistant %s %s: %w", method, path, err)
	}
	return nil
}
