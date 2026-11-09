package homeassistant

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"time"

	"jarvis/client/internal/actions"
	"jarvis/client/internal/commands"
)

// Entities returns the entities exposed to the Home Assistant voice assistant,
// with their state and attributes. Unavailable entities are included; the
// catalog generator decides whether they can produce actions.
func (c *Client) Entities(ctx context.Context) ([]actions.Entity, error) {
	exposed, err := c.exposedEntities(ctx)
	if err != nil {
		return nil, err
	}
	var states []struct {
		EntityID   string         `json:"entity_id"`
		State      string         `json:"state"`
		Attributes map[string]any `json:"attributes"`
	}
	if err := c.rest(ctx, http.MethodGet, "/api/states", nil, &states); err != nil {
		return nil, err
	}
	entities := make([]actions.Entity, 0, len(states))
	for _, state := range states {
		m := entityID.FindStringSubmatch(state.EntityID)
		if m == nil || !exposed[state.EntityID][assistant] {
			continue
		}
		attributes := state.Attributes
		if attributes == nil {
			attributes = map[string]any{}
		}
		name := strings.TrimSpace(stringAttribute(attributes, "friendly_name"))
		if name == "" {
			name = strings.ReplaceAll(m[2], "_", " ")
		}
		entities = append(entities, actions.Entity{
			EntityID:   state.EntityID,
			Domain:     m[1],
			Name:       name,
			State:      state.State,
			Attributes: attributes,
		})
	}
	slices.SortFunc(entities, func(a, b actions.Entity) int {
		return strings.Compare(a.EntityID, b.EntityID)
	})
	return entities, nil
}

// Actions returns the generated action catalog of the exposed entities.
func (c *Client) Actions(ctx context.Context) ([]actions.Action, error) {
	entities, err := c.Entities(ctx)
	if err != nil {
		return nil, err
	}
	return actions.Generate(entities, time.Now()).Actions, nil
}

// ExecuteAction calls the Home Assistant service of one catalog action. It
// validates the action again, so a hand-edited catalog cannot name another
// service or send other arguments.
func (c *Client) ExecuteAction(ctx context.Context, action actions.Action) error {
	if err := actions.Validate(action); err != nil {
		return err
	}
	domain, service, ok := strings.Cut(action.Service, ".")
	if !ok {
		return ErrNotAllowed
	}
	body := make(map[string]any, len(action.Data)+1)
	body["entity_id"] = action.EntityID
	for key, value := range action.Data {
		body[key] = value
	}
	return c.rest(ctx, http.MethodPost, "/api/services/"+domain+"/"+service, body, nil)
}

func stringAttribute(attributes map[string]any, key string) string {
	value, ok := attributes[key]
	if !ok {
		return ""
	}
	text, _ := value.(string)
	return text
}

// powerDevices filters the entities that the assistant may switch directly. It
// keeps the name fallback of Devices and marks unavailable lights and switches.
func powerDevices(entities []actions.Entity) []commands.Device {
	var devices []commands.Device
	for _, entity := range entities {
		if !powerDomains[entity.Domain] {
			continue
		}
		devices = append(devices, commands.Device{
			ID:          entity.EntityID,
			Name:        entity.Name,
			Unavailable: entity.State == "unavailable",
		})
	}
	slices.SortFunc(devices, func(a, b commands.Device) int {
		return strings.Compare(a.Name, b.Name)
	})
	return devices
}
