package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/commands"
)

func completionBody(t *testing.T, content string) string {
	t.Helper()
	b, err := json.Marshal(map[string]any{
		"choices": []any{map[string]any{"message": map[string]string{"content": content}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func testServer(t *testing.T, content string, requests chan<- request) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" {
			http.NotFound(w, r)
			return
		}
		var req request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if requests != nil {
			requests <- req
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(completionBody(t, content)))
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestInterpret(t *testing.T) {
	requests := make(chan request, 1)
	srv := testServer(t, `{"commands":[{"action":"turn_on","device":"light.kitchen"},{"action":"turn_off","device":"light.lounge"}]}`, requests)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	devices := []commands.Device{
		{ID: "light.kitchen", Name: "la luz de la cocina"},
		{ID: "light.lounge", Name: "la luz del salón"},
	}
	cmds, err := c.Interpret(context.Background(), "enciende la cocina y apaga el salón", devices)
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	want := []commands.Command{
		{Kind: commands.KindTurnOn, Device: devices[0]},
		{Kind: commands.KindTurnOff, Device: devices[1]},
	}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("commands %+v, want %+v", cmds, want)
	}

	req := <-requests
	if req.Model != "local" || req.Temperature != 0 || req.MaxTokens != maxTokens {
		t.Errorf("request %+v", req)
	}
	if len(req.Messages) != 2 || req.Messages[1].Role != "user" || req.Messages[1].Content != "enciende la cocina y apaga el salón" {
		t.Fatalf("messages %+v", req.Messages)
	}
	system := req.Messages[0].Content
	for _, want := range []string{"light.kitchen = la luz de la cocina", "turn_on", `{"commands": []}`, `{"action": "turn_on", "device": "light.kitchen"}`} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt does not contain %q:\n%s", want, system)
		}
	}
	schema, err := json.Marshal(req.ResponseFormat.JSONSchema.Schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{`"turn_on"`, `"light.kitchen"`, `"maxItems":4`} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("schema does not contain %s:\n%s", want, schema)
		}
	}
}

func TestInterpretAnswers(t *testing.T) {
	devices := []commands.Device{{ID: "light.kitchen", Name: "la luz de la cocina"}}
	tests := []struct {
		name    string
		content string
		wantErr string
	}{
		{"no command", `{"commands":[]}`, ""},
		{"unknown device", `{"commands":[{"action":"turn_on","device":"light.other"}]}`, "unknown device"},
		{"unknown action", `{"commands":[{"action":"explode","device":"light.kitchen"}]}`, "unknown action"},
		{"too many", `{"commands":[{"action":"turn_on","device":"light.kitchen"},{"action":"turn_on","device":"light.kitchen"},{"action":"turn_on","device":"light.kitchen"},{"action":"turn_on","device":"light.kitchen"},{"action":"turn_on","device":"light.kitchen"}]}`, "at most"},
		{"malformed", `not json`, "invalid JSON"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := testServer(t, tt.content, nil)
			c := New(Config{URL: srv.URL, Timeout: time.Second})
			cmds, err := c.Interpret(context.Background(), "enciende la luz", devices)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("Interpret: %v", err)
				}
				if len(cmds) != 0 {
					t.Errorf("commands %+v, want none", cmds)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %v, want one that mentions %q", err, tt.wantErr)
			}
		})
	}
}

func TestInterpretWithoutDevicesSkipsRequest(t *testing.T) {
	called := false
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		called = true
	}))
	defer srv.Close()
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	cmds, err := c.Interpret(context.Background(), "enciende la luz", nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(cmds) != 0 {
		t.Errorf("commands %+v, want none", cmds)
	}
	if called {
		t.Error("Interpret called the server without devices")
	}
}

func TestInterpretWithoutPowerVerbReasons(t *testing.T) {
	requests := make(chan request, 1)
	srv := testServer(t, `{"commands":[{"action":"turn_on","device":"light.kitchen"}]}`, requests)
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	devices := []commands.Device{{ID: "light.kitchen", Name: "la luz de la cocina"}}
	cmds, err := c.Interpret(context.Background(), "hace frío", devices)
	if err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	want := []commands.Command{{Kind: commands.KindTurnOn, Device: devices[0]}}
	if !reflect.DeepEqual(cmds, want) {
		t.Errorf("commands %+v, want %+v", cmds, want)
	}
	if req := <-requests; req.Messages[1].Content != "hace frío" {
		t.Errorf("user message %q", req.Messages[1].Content)
	}
}

func TestInterpretUsesRoomForAState(t *testing.T) {
	requests := make(chan request, 1)
	srv := testServer(t, `{"commands":[]}`, requests)
	c := New(Config{URL: srv.URL, Timeout: time.Second, Room: "Salón"})
	devices := []commands.Device{
		{ID: "light.kitchen", Name: "la luz de la cocina"},
		{ID: "light.lounge", Name: "la luz del salón"},
	}
	if _, err := c.Interpret(context.Background(), "hay demasiada luz aquí", devices); err != nil {
		t.Fatalf("Interpret: %v", err)
	}
	system := (<-requests).Messages[0].Content
	for _, want := range []string{
		`The assistant is in the room "Salón"`,
		`Usuario: hay demasiada luz aquí`,
		`{"action": "turn_off", "device": "light.lounge"}`,
	} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt does not contain %q:\n%s", want, system)
		}
	}
}

func TestInterpretHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	}))
	defer srv.Close()
	c := New(Config{URL: srv.URL, Timeout: time.Second})
	devices := []commands.Device{{ID: "light.kitchen", Name: "la luz de la cocina"}}
	if _, err := c.Interpret(context.Background(), "enciende la luz", devices); err == nil || !strings.Contains(err.Error(), "503") {
		t.Errorf("error %v, want an HTTP 503 error", err)
	}
}

func TestInterpretEmptyAnswer(t *testing.T) {
	for name, body := range map[string]string{
		"no choices": `{"choices":[]}`,
		"no content": completionBody(t, ""),
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer srv.Close()
			c := New(Config{URL: srv.URL, Timeout: time.Second})
			devices := []commands.Device{{ID: "light.kitchen", Name: "la luz de la cocina"}}
			if _, err := c.Interpret(context.Background(), "enciende la luz", devices); err == nil {
				t.Fatal("Interpret returned no error")
			}
		})
	}
}
