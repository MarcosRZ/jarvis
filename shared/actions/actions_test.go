package actions

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func entity(id, domain, name, state string, attributes map[string]any) Entity {
	if attributes == nil {
		attributes = map[string]any{}
	}
	if name != "" {
		attributes["friendly_name"] = name
	}
	return Entity{EntityID: id, Domain: domain, Name: name, State: state, Attributes: attributes}
}

func ids(catalog Catalog) []string {
	out := make([]string, len(catalog.Actions))
	for i, action := range catalog.Actions {
		out[i] = action.ID
	}
	return out
}

func TestGenerateControlsCommonDomains(t *testing.T) {
	now := time.Date(2026, 11, 10, 9, 0, 0, 0, time.UTC)
	entities := []Entity{
		entity("light.salon", "light", "La luz del salón", "on", nil),
		entity("switch.enchufe", "switch", "El enchufe", "off", nil),
		entity("scene.cine", "scene", "Cine", "unknown", nil),
		entity("script.buenos_dias", "script", "Buenos días", "off", nil),
		entity("cover.persiana", "cover", "Persiana del salón", "closed", nil),
		entity("lock.puerta", "lock", "Puerta de entrada", "locked", nil),
		entity("climate.aire", "climate", "El aire acondicionado", "off", map[string]any{
			"hvac_modes": []any{"off", "heat", "cool"},
		}),
		entity("media_player.tele", "media_player", "TV salón", "off", nil),
		entity("vacuum.robot", "vacuum", "Roomba", "docked", nil),
		entity("fan.ventilador", "fan", "Ventilador", "off", map[string]any{
			"preset_modes": []any{"low", "high"},
		}),
		entity("input_select.escena", "input_select", "Escena", "cine", map[string]any{
			"options": []any{"cine", "noche"},
		}),
		entity("timer.hora", "timer", "Temporizador", "idle", nil),
	}
	catalog := Generate(entities, now)
	if catalog.Version != Version || !catalog.Generated.Equal(now) {
		t.Errorf("catalog header %+v", catalog)
	}
	got := ids(catalog)
	for _, want := range []string{
		"light.salon.turn_on", "light.salon.turn_off", "light.salon.toggle",
		"switch.enchufe.turn_on", "scene.cine.turn_on", "script.buenos_dias.turn_on",
		"cover.persiana.open", "cover.persiana.close", "cover.persiana.stop",
		"lock.puerta.lock", "lock.puerta.unlock",
		"climate.aire.turn_on", "climate.aire.mode_heat", "climate.aire.mode_cool",
		"media_player.tele.play", "media_player.tele.mute",
		"vacuum.robot.start", "vacuum.robot.return_to_base",
		"fan.ventilador.preset_low", "fan.ventilador.preset_high",
		"input_select.escena.option_cine", "input_select.escena.option_noche",
		"timer.hora.start", "timer.hora.cancel",
	} {
		if !slices.Contains(got, want) {
			t.Errorf("catalog has no %s", want)
		}
	}
	byID := map[string]Action{}
	for _, action := range catalog.Actions {
		byID[action.ID] = action
		if err := Validate(action); err != nil {
			t.Errorf("generated action %s is invalid: %v", action.ID, err)
		}
	}
	if got := byID["climate.aire.mode_heat"]; got.Service != "climate.set_hvac_mode" || got.Data["hvac_mode"] != "heat" {
		t.Errorf("climate action %+v", got)
	}
	if got := byID["light.salon.turn_on"]; !strings.Contains(got.Name, "luz del salón") || !slices.Contains(got.Aliases, "lampara") {
		t.Errorf("light action %+v", got)
	}
	if got := byID["media_player.tele.play"]; !strings.Contains(got.Description, "tele") {
		t.Errorf("description %q should mention an alias", got.Description)
	}
}

func TestGenerateSkipsReadOnlyAndUnavailableEntities(t *testing.T) {
	entities := []Entity{
		entity("sensor.temperatura", "sensor", "Temperatura", "20", nil),
		entity("weather.casa", "weather", "Casa", "rainy", nil),
		entity("camera.entrada", "camera", "Entrada", "idle", nil),
		entity("light.apagada", "light", "Luz apagada", "unavailable", nil),
		entity("lock.rota", "lock", "Cerradura rota", "unavailable", nil),
	}
	if catalog := Generate(entities, time.Now()); len(catalog.Actions) != 0 {
		t.Errorf("actions %v, want none", ids(catalog))
	}
}

func TestGenerateSkipsUnknownDomains(t *testing.T) {
	catalog := Generate([]Entity{
		entity("sensor.x", "sensor", "X", "1", nil),
		entity("update.core", "update", "Core", "on", nil),
		entity("todo.lista", "todo", "Lista", "0", nil),
	}, time.Now())
	if len(catalog.Actions) != 0 {
		t.Errorf("actions %v, want none", ids(catalog))
	}
}

func TestActionIDsAreStableAndUnique(t *testing.T) {
	e := []Entity{
		entity("light.salon", "light", "Luz", "on", nil),
		entity("switch.salon", "switch", "Luz", "off", nil),
	}
	first := Generate(e, time.Now())
	second := Generate(e, time.Now().Add(time.Hour))
	if !slices.Equal(ids(first), ids(second)) {
		t.Errorf("IDs changed: %v then %v", ids(first), ids(second))
	}
	seen := map[string]bool{}
	for _, action := range first.Actions {
		if seen[action.ID] {
			t.Errorf("duplicate ID %s", action.ID)
		}
		seen[action.ID] = true
	}
}

func TestValidateRejectsUnsafeActions(t *testing.T) {
	valid := Action{
		ID: "light.salon.turn_on", Name: "Encender", EntityID: "light.salon",
		Domain: "light", Service: "light.turn_on",
	}
	if err := Validate(valid); err != nil {
		t.Fatalf("valid action: %v", err)
	}
	tests := map[string]Action{
		"empty ID":        {Name: "x", EntityID: "light.salon", Domain: "light", Service: "light.turn_on"},
		"bad entity":      {ID: "x", Name: "x", EntityID: "light", Domain: "light", Service: "light.turn_on"},
		"wrong domain":    {ID: "x", Name: "x", EntityID: "light.salon", Domain: "switch", Service: "switch.turn_on"},
		"wrong service":   {ID: "x", Name: "x", EntityID: "light.salon", Domain: "light", Service: "script.turn_on"},
		"unknown service": {ID: "x", Name: "x", EntityID: "light.salon", Domain: "light", Service: "light.explode"},
		"unexpected data": {ID: "x", Name: "x", EntityID: "light.salon", Domain: "light", Service: "light.turn_on", Data: map[string]any{"brightness": 1}},
	}
	for name, action := range tests {
		if err := Validate(action); !errors.Is(err, ErrBadAction) {
			t.Errorf("%s: error %v, want ErrBadAction", name, err)
		}
	}
}

func TestSaveWritesReadableCatalog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "actions.json")
	catalog := Generate([]Entity{entity("light.salon", "light", "Luz", "on", nil)}, time.Now())
	if err := Save(path, catalog); err != nil {
		t.Fatalf("Save: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o644 {
		t.Errorf("mode %v, want 0644", info.Mode().Perm())
	}
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var got Catalog
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(got.Actions) != 3 || got.Actions[0].ID == "" {
		t.Errorf("catalog %+v", got)
	}
	if err := Save("", catalog); err == nil {
		t.Error("Save with an empty path did not fail")
	}
}
