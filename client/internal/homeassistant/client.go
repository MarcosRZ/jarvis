// Package homeassistant reads the devices exposed to voice assistants in Home
// Assistant, switches lights and switches, and forwards any other sentence to
// the Home Assistant conversation agent. It implements commands.DeviceController
// and commands.Conversation.
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
	// ConversationTimeout bounds a conversation request. An LLM agent can take
	// longer than a light call. Zero uses Timeout.
	ConversationTimeout time.Duration
	// Language is the language of the conversation input, for example "es".
	// Empty uses the Home Assistant server language.
	Language string
	// AgentID selects a conversation agent, for example "conversation.ollama".
	// Empty uses the Home Assistant default agent.
	AgentID string
}

type Client struct {
	cfg              Config
	http             *http.Client
	conversationHTTP *http.Client
}

func New(cfg Config) *Client {
	cfg.URL = strings.TrimRight(cfg.URL, "/")
	if cfg.ConversationTimeout <= 0 {
		cfg.ConversationTimeout = cfg.Timeout
	}
	return &Client{
		cfg:              cfg,
		http:             &http.Client{Timeout: cfg.Timeout},
		conversationHTTP: &http.Client{Timeout: cfg.ConversationTimeout},
	}
}

// Devices returns the lights and switches exposed to the Home Assistant voice
// assistant, sorted by name. The name is the friendly name, or the object ID
// with spaces when there is none. A device in the state "unavailable" is in
// the list, marked Unavailable.
func (c *Client) Devices(ctx context.Context) ([]commands.Device, error) {
	entities, err := c.Entities(ctx)
	if err != nil {
		return nil, err
	}
	return powerDevices(entities), nil
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

// Converse sends text to the Home Assistant conversation agent. handled is
// false when the agent did not understand the sentence, so the caller can use
// its own fallback. The agent controls any entity exposed to it, not only the
// lights and switches of Devices.
func (c *Client) Converse(ctx context.Context, text string) (string, bool, error) {
	body := map[string]any{"text": text}
	if c.cfg.Language != "" {
		body["language"] = c.cfg.Language
	}
	if c.cfg.AgentID != "" {
		body["agent_id"] = c.cfg.AgentID
	}
	var result struct {
		Response struct {
			ResponseType string `json:"response_type"`
			Speech       struct {
				Plain struct {
					Speech string `json:"speech"`
				} `json:"plain"`
			} `json:"speech"`
		} `json:"response"`
	}
	if err := c.restWith(ctx, c.conversationHTTP, http.MethodPost, "/api/conversation/process", body, &result); err != nil {
		return "", false, err
	}
	if result.Response.ResponseType == "error" {
		return "", false, nil
	}
	speech := strings.TrimSpace(result.Response.Speech.Plain.Speech)
	if speech == "" {
		if result.Response.ResponseType == "action_done" {
			return commands.ReplyDone, true, nil
		}
		return "", false, nil
	}
	return speech, true, nil
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
	return c.restWith(ctx, c.http, method, path, body, out)
}

func (c *Client) restWith(ctx context.Context, client *http.Client, method, path string, body, out any) error {
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
	resp, err := client.Do(req)
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
