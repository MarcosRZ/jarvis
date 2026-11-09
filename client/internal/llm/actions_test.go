package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/actions"
	"jarvis/client/internal/commands"
)

func actionCatalog() []actions.Action {
	return []actions.Action{
		{
			ID: "light.lounge.turn_on", Name: "Encender la luz del salón",
			Description: "Encender la luz del salón (light.lounge; tambien: luz, lampara)",
			EntityID:    "light.lounge", Domain: "light", Service: "light.turn_on",
			Aliases: []string{"luz", "lampara"},
		},
		{
			ID: "scene.cine.turn_on", Name: "Activar Cine",
			Description: "Activar Cine (scene.cine; tambien: escena)",
			EntityID:    "scene.cine", Domain: "scene", Service: "scene.turn_on",
			Aliases: []string{"escena"},
		},
	}
}

func TestInterpretActions(t *testing.T) {
	requests := make(chan request, 1)
	srv := testServer(t, `{"commands":[{"action":"scene.cine.turn_on"}],"reply":""}`, requests)
	c := New(Config{URL: srv.URL, Timeout: time.Second})

	got, err := c.InterpretActions(context.Background(), "pon la escena cine", actionCatalog(), "light.lounge: on")
	if err != nil {
		t.Fatalf("InterpretActions: %v", err)
	}
	if len(got.Commands) != 1 || got.Commands[0].Kind != commands.KindAction || got.Commands[0].Action.ID != "scene.cine.turn_on" {
		t.Fatalf("interpretation %+v", got)
	}
	if got.Reply != "" {
		t.Errorf("reply %q, want empty", got.Reply)
	}

	req := <-requests
	system := req.Messages[0].Content
	for _, want := range []string{
		"scene.cine.turn_on", "Activar Cine", "luz", "lampara",
		"enciende luz", "encender, poner and activar mean turn_on",
		"one or more actions", "quiero encender la luz del salón y activar Cine",
		`{"action": "<id>"}`, "light.lounge: on",
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt does not contain %q:\n%s", want, system)
		}
	}
	schema, err := json.Marshal(req.ResponseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"scene.cine.turn_on"`, `"light.lounge.turn_on"`, `"maxItems":4`, `"reply"`} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("schema does not contain %s:\n%s", want, schema)
		}
	}
}

func TestInterpretActionsReturnsAQuestionReply(t *testing.T) {
	srv := testServer(t, `{"commands":[],"reply":"Mañana llueve."}`, nil)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	got, err := c.InterpretActions(context.Background(), "¿va a llover mañana?", actionCatalog(), "weather.casa: rainy")
	if err != nil {
		t.Fatalf("InterpretActions: %v", err)
	}
	if len(got.Commands) != 0 || got.Reply != "Mañana llueve." {
		t.Errorf("interpretation %+v", got)
	}
}

func TestInterpretActionsRejectsAnUnknownID(t *testing.T) {
	srv := testServer(t, `{"commands":[{"action":"scene.other.turn_on"}],"reply":""}`, nil)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	if _, err := c.InterpretActions(context.Background(), "pon la escena", actionCatalog(), ""); err == nil || !strings.Contains(err.Error(), "unknown action") {
		t.Fatalf("error %v, want an unknown action", err)
	}
}

func TestInterpretActionsReturnsSeveralActions(t *testing.T) {
	srv := testServer(t, `{"commands":[{"action":"scene.cine.turn_on"},{"action":"light.lounge.turn_on"},{"action":"scene.cine.turn_on"}],"reply":""}`, nil)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	got, err := c.InterpretActions(context.Background(), "pon la escena y enciende la luz", actionCatalog(), "")
	if err != nil {
		t.Fatalf("InterpretActions: %v", err)
	}
	if len(got.Commands) != 2 || got.Commands[0].Action.ID != "scene.cine.turn_on" || got.Commands[1].Action.ID != "light.lounge.turn_on" {
		t.Errorf("interpretation %+v, want both actions in order without the duplicate", got)
	}
}

func TestInterpretActionsRejectsTooManyActions(t *testing.T) {
	srv := testServer(t, `{"commands":[{"action":"scene.cine.turn_on"},{"action":"light.lounge.turn_on"},{"action":"scene.cine.turn_on"},{"action":"light.lounge.turn_on"},{"action":"scene.cine.turn_on"}],"reply":""}`, nil)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	if _, err := c.InterpretActions(context.Background(), "haz todo", actionCatalog(), ""); err == nil || !strings.Contains(err.Error(), "at most 4") {
		t.Fatalf("error %v, want at most four actions", err)
	}
}

func TestInterpretActionsWithoutCatalogSkipsRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { called = true }))
	t.Cleanup(srv.Close)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	got, err := c.InterpretActions(context.Background(), "pon la escena", nil, "")
	if err != nil {
		t.Fatalf("InterpretActions: %v", err)
	}
	if len(got.Commands) != 0 || got.Reply != "" || called {
		t.Errorf("interpretation %+v, called %v", got, called)
	}
}
