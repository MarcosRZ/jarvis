package homeassistant

import (
	"context"
	"errors"
	"slices"
	"testing"

	"jarvis/client/internal/actions"
)

func TestEntitiesReturnsTheExposedEntities(t *testing.T) {
	f := &fakeHA{
		states: []state{
			named("light.kitchen", "La luz de la cocina"),
			{
				EntityID: "climate.aire",
				State:    "off",
				Attributes: map[string]any{
					"friendly_name": "El aire acondicionado",
					"hvac_modes":    []any{"off", "heat"},
				},
			},
			named("lock.hidden", "La puerta"),
		},
		exposed: map[string]map[string]bool{
			"light.kitchen": {"conversation": true},
			"climate.aire":  {"conversation": true},
			"lock.hidden":   {"cloud.alexa": true},
		},
	}
	entities, err := newClient(f.start(t)).Entities(context.Background())
	if err != nil {
		t.Fatalf("Entities: %v", err)
	}
	if len(entities) != 2 {
		t.Fatalf("entities %+v, want 2", entities)
	}
	if entities[0].EntityID != "climate.aire" || entities[0].Domain != "climate" || entities[0].Name != "El aire acondicionado" {
		t.Errorf("climate entity %+v", entities[0])
	}
	if modes, ok := entities[0].Attributes["hvac_modes"].([]any); !ok || len(modes) != 2 {
		t.Errorf("attributes %+v", entities[0].Attributes)
	}
	if entities[1].EntityID != "light.kitchen" || entities[1].Name != "La luz de la cocina" {
		t.Errorf("light entity %+v", entities[1])
	}
}

func TestActionsGeneratesTheCatalogFromExposedEntities(t *testing.T) {
	f := &fakeHA{
		states: []state{
			named("light.kitchen", "La luz de la cocina"),
			named("lock.door", "La puerta"),
			named("sensor.temperature", "Temperatura"),
			named("lock.hidden", "Otra puerta"),
		},
		exposed: map[string]map[string]bool{
			"light.kitchen":      {"conversation": true},
			"lock.door":          {"conversation": true},
			"sensor.temperature": {"conversation": true},
			"lock.hidden":        {"cloud.alexa": true},
		},
	}
	catalog, err := newClient(f.start(t)).Actions(context.Background())
	if err != nil {
		t.Fatalf("Actions: %v", err)
	}
	var ids []string
	for _, action := range catalog {
		ids = append(ids, action.ID)
	}
	for _, want := range []string{"light.kitchen.turn_on", "light.kitchen.turn_off", "lock.door.lock", "lock.door.unlock"} {
		if !slices.Contains(ids, want) {
			t.Errorf("catalog has no %s: %v", want, ids)
		}
	}
	for _, unwanted := range []string{"lock.hidden.lock", "sensor.temperature.turn_on"} {
		if slices.Contains(ids, unwanted) {
			t.Errorf("catalog has %s: %v", unwanted, ids)
		}
	}
}

func TestExecuteActionCallsTheServiceWithItsData(t *testing.T) {
	f := &fakeHA{}
	action := actions.Action{
		ID: "climate.aire.mode_heat", Name: "Poner el aire en modo calor",
		Description: "Poner el aire en modo calor", EntityID: "climate.aire",
		Domain: "climate", Service: "climate.set_hvac_mode",
		Data: map[string]any{"hvac_mode": "heat"},
	}
	if err := newClient(f.start(t)).ExecuteAction(context.Background(), action); err != nil {
		t.Fatalf("ExecuteAction: %v", err)
	}
	if !slices.Equal(f.calls, []string{"climate/set_hvac_mode"}) {
		t.Errorf("calls %v", f.calls)
	}
	if len(f.bodies) != 1 || f.bodies[0]["entity_id"] != "climate.aire" || f.bodies[0]["hvac_mode"] != "heat" {
		t.Errorf("body %v", f.bodies)
	}
}

func TestExecuteActionRejectsAnEditedCatalog(t *testing.T) {
	f := &fakeHA{}
	c := newClient(f.start(t))
	base := actions.Action{
		ID: "light.kitchen.turn_on", Name: "Encender la luz",
		EntityID: "light.kitchen", Domain: "light", Service: "light.turn_on",
	}
	tests := map[string]actions.Action{
		"other service":   func() actions.Action { a := base; a.Service = "light.explode"; return a }(),
		"other domain":    func() actions.Action { a := base; a.Service = "lock.unlock"; return a }(),
		"bad entity":      func() actions.Action { a := base; a.EntityID = "light.kitchen/../x"; return a }(),
		"unexpected data": func() actions.Action { a := base; a.Data = map[string]any{"brightness": 10}; return a }(),
		"empty action ID": func() actions.Action { a := base; a.ID = ""; return a }(),
	}
	for name, action := range tests {
		err := c.ExecuteAction(context.Background(), action)
		if !errors.Is(err, actions.ErrBadAction) {
			t.Errorf("%s: error %v, want ErrBadAction", name, err)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("calls %v for rejected actions", f.calls)
	}
}
