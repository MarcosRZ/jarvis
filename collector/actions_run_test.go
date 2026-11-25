package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	actioncatalog "jarvis/actions"
)

// serviceServer records every Home Assistant service call.
type serviceServer struct {
	mu     sync.Mutex
	paths  []string
	auth   []string
	bodies []map[string]any
}

func (s *serviceServer) handler(w http.ResponseWriter, r *http.Request) {
	var body map[string]any
	_ = json.NewDecoder(r.Body).Decode(&body)
	s.mu.Lock()
	s.paths = append(s.paths, r.URL.Path)
	s.auth = append(s.auth, r.Header.Get("Authorization"))
	s.bodies = append(s.bodies, body)
	s.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	fmt.Fprint(w, `[]`)
}

func (s *serviceServer) calls() ([]string, []string, []map[string]any) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.paths...), append([]string(nil), s.auth...), append([]map[string]any(nil), s.bodies...)
}

func TestRunActionCallsTheCatalogService(t *testing.T) {
	recorder := &serviceServer{}
	server := httptest.NewServer(http.HandlerFunc(recorder.handler))
	defer server.Close()

	home := newHomeAssistantClient(homeAssistantConfig{URL: server.URL, Token: "token"}, time.Second)
	app := newCollector(config{RequestTimeout: time.Second}, &fakeDocker{}, home)
	app.storeActions(ActionsSnapshot{
		Source: SourceStatus{State: SourceOK},
		Actions: []actioncatalog.Action{
			{
				ID: "light.salon.turn_on", Name: "Encender Luz salon",
				EntityID: "light.salon", Domain: "light", Service: "light.turn_on",
				Data: map[string]any{"brightness": float64(128)},
			},
		},
	})

	action, err := app.RunAction(context.Background(), "light.salon.turn_on")
	if err != nil {
		t.Fatalf("RunAction: %v", err)
	}
	if action.ID != "light.salon.turn_on" {
		t.Fatalf("action = %+v", action)
	}
	paths, auth, bodies := recorder.calls()
	if len(paths) != 1 || paths[0] != "/api/services/light/turn_on" {
		t.Fatalf("paths = %v", paths)
	}
	if len(auth) != 1 || auth[0] != "Bearer token" {
		t.Fatalf("auth = %v", auth)
	}
	if bodies[0]["entity_id"] != "light.salon" || bodies[0]["brightness"] != float64(128) {
		t.Fatalf("body = %v", bodies[0])
	}

	// An ID outside the catalog never reaches Home Assistant.
	if _, err := app.RunAction(context.Background(), "light.salon.turn_off"); !errors.Is(err, ErrActionNotFound) {
		t.Fatalf("unknown action error = %v, want ErrActionNotFound", err)
	}
	if paths, _, _ := recorder.calls(); len(paths) != 1 {
		t.Fatalf("an unknown action reached Home Assistant: %v", paths)
	}
}

func TestRunActionRejectsAnInvalidCatalogEntry(t *testing.T) {
	recorder := &serviceServer{}
	server := httptest.NewServer(http.HandlerFunc(recorder.handler))
	defer server.Close()

	home := newHomeAssistantClient(homeAssistantConfig{URL: server.URL, Token: "token"}, time.Second)
	app := newCollector(config{RequestTimeout: time.Second}, &fakeDocker{}, home)
	app.storeActions(ActionsSnapshot{
		Source: SourceStatus{State: SourceOK},
		Actions: []actioncatalog.Action{
			// The service domain does not match the entity domain.
			{ID: "light.salon.turn_on", EntityID: "light.salon", Domain: "light", Service: "script.turn_on"},
		},
	})
	if _, err := app.RunAction(context.Background(), "light.salon.turn_on"); err == nil {
		t.Fatal("a cross-domain service must be rejected")
	}
	if paths, _, _ := recorder.calls(); len(paths) != 0 {
		t.Fatalf("an invalid action reached Home Assistant: %v", paths)
	}
}

func TestRunActionWithoutHomeAssistant(t *testing.T) {
	app := newCollector(config{RequestTimeout: time.Second}, &fakeDocker{}, nil)
	if _, err := app.RunAction(context.Background(), "light.salon.turn_on"); !errors.Is(err, ErrActionsUnavailable) {
		t.Fatalf("error = %v, want ErrActionsUnavailable", err)
	}
}
