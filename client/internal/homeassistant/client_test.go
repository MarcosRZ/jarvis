package homeassistant

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/coder/websocket"

	"jarvis/client/internal/commands"
)

const testToken = "test-token"

type state struct {
	EntityID   string         `json:"entity_id"`
	State      string         `json:"state"`
	Attributes map[string]any `json:"attributes"`
}

// fakeHA serves the parts of the Home Assistant API that the client uses.
type fakeHA struct {
	mu             sync.Mutex
	states         []state
	exposed        map[string]map[string]bool
	calls          []string
	bodies         []map[string]any
	failCall       bool
	wsAuth         string
	conversation   map[string]any
	conversationAt []map[string]any
	converseDelay  time.Duration
	failConverse   bool
	forecasts      map[string]any
}

func named(id, name string) state {
	return state{EntityID: id, State: "off", Attributes: map[string]any{"friendly_name": name}}
}

func (f *fakeHA) start(t *testing.T) string {
	t.Helper()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/states", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		_ = json.NewEncoder(w).Encode(f.states)
	})
	mux.HandleFunc("POST /api/services/{domain}/{service}", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		forecasts := f.forecasts
		f.calls = append(f.calls, r.PathValue("domain")+"/"+r.PathValue("service"))
		f.bodies = append(f.bodies, body)
		fail := f.failCall
		f.mu.Unlock()
		if r.PathValue("domain") == "weather" && r.PathValue("service") == "get_forecasts" {
			if forecasts == nil {
				http.Error(w, "no forecast", http.StatusBadRequest)
				return
			}
			_ = json.NewEncoder(w).Encode(forecasts)
			return
		}
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte("[]"))
	})
	mux.HandleFunc("POST /api/conversation/process", func(w http.ResponseWriter, r *http.Request) {
		if !authorized(w, r) {
			return
		}
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		f.mu.Lock()
		f.conversationAt = append(f.conversationAt, body)
		response := f.conversation
		fail := f.failConverse
		f.mu.Unlock()
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if delay := f.converseDelay; delay > 0 {
			time.Sleep(delay)
		}
		if response == nil {
			response = map[string]any{
				"response": map[string]any{
					"response_type": "action_done",
					"speech":        map[string]any{"plain": map[string]any{"speech": "Hecho."}},
				},
			}
		}
		_ = json.NewEncoder(w).Encode(response)
	})
	mux.HandleFunc("GET /api/websocket", f.websocket)
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL
}

func authorized(w http.ResponseWriter, r *http.Request) bool {
	if r.Header.Get("Authorization") != "Bearer "+testToken {
		http.Error(w, "401: Unauthorized", http.StatusUnauthorized)
		return false
	}
	return true
}

func (f *fakeHA) websocket(w http.ResponseWriter, r *http.Request) {
	c, err := websocket.Accept(w, r, nil)
	if err != nil {
		return
	}
	defer c.CloseNow()
	ctx := r.Context()
	send := func(v any) { b, _ := json.Marshal(v); _ = c.Write(ctx, websocket.MessageText, b) }
	read := func() map[string]any {
		_, b, err := c.Read(ctx)
		if err != nil {
			return nil
		}
		var m map[string]any
		_ = json.Unmarshal(b, &m)
		return m
	}
	send(map[string]any{"type": "auth_required"})
	auth := read()
	f.mu.Lock()
	f.wsAuth, _ = auth["access_token"].(string)
	f.mu.Unlock()
	if auth["access_token"] != testToken {
		send(map[string]any{"type": "auth_invalid", "message": "Invalid access token"})
		return
	}
	send(map[string]any{"type": "auth_ok"})
	for msg := read(); msg != nil; msg = read() {
		id := msg["id"]
		if msg["type"] != "homeassistant/expose_entity/list" {
			send(map[string]any{"id": id, "type": "result", "success": false, "error": map[string]any{"code": "unknown_command", "message": "Unknown command."}})
			continue
		}
		f.mu.Lock()
		result := map[string]any{"exposed_entities": f.exposed}
		f.mu.Unlock()
		send(map[string]any{"id": id, "type": "result", "success": true, "result": result})
	}
}

func newClient(url string) *Client {
	return New(Config{URL: url, Token: testToken, Timeout: time.Second})
}

func TestDevicesListsExposedLightsAndSwitches(t *testing.T) {
	f := &fakeHA{
		states: []state{
			named("light.kitchen", "La luz de la cocina"),
			named("switch.fan", "El ventilador"),
			named("light.hidden", "Luz oculta"),
			named("lock.door", "La puerta"),
			named("light.voice_off", "Luz sin voz"),
			{EntityID: "light.no_name", State: "on", Attributes: map[string]any{}},
			{EntityID: "light.led", State: "unavailable", Attributes: map[string]any{"friendly_name": "El LED"}},
		},
		exposed: map[string]map[string]bool{
			"light.kitchen":   {"conversation": true},
			"switch.fan":      {"conversation": true, "cloud.alexa": true},
			"lock.door":       {"conversation": true},
			"light.voice_off": {"cloud.alexa": true},
			"light.no_name":   {"conversation": true},
			"light.led":       {"conversation": true},
		},
	}
	devices, err := newClient(f.start(t)).Devices(context.Background())
	if err != nil {
		t.Fatalf("Devices: %v", err)
	}
	want := []commands.Device{
		{ID: "light.led", Name: "El LED", Unavailable: true},
		{ID: "switch.fan", Name: "El ventilador"},
		{ID: "light.kitchen", Name: "La luz de la cocina"},
		{ID: "light.no_name", Name: "no name"},
	}
	if !slices.Equal(devices, want) {
		t.Errorf("devices %+v, want %+v", devices, want)
	}
}

func TestDevicesRejectsBadToken(t *testing.T) {
	c := New(Config{URL: (&fakeHA{}).start(t), Token: "wrong", Timeout: time.Second})
	if _, err := c.Devices(context.Background()); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error %v, want ErrUnauthorized", err)
	}
}

func TestDevicesServiceUnavailable(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	if _, err := newClient(url).Devices(context.Background()); err == nil {
		t.Fatal("no error from a closed server")
	}
}

func TestContextIncludesExposedStatesAndWeatherForecast(t *testing.T) {
	f := &fakeHA{
		states: []state{
			named("light.kitchen", "La luz de la cocina"),
			{
				EntityID: "weather.forecast_casa",
				State:    "partlycloudy",
				Attributes: map[string]any{
					"friendly_name": "Forecast Casa",
					"temperature":   9.2,
					"humidity":      91,
				},
			},
			named("lock.door", "La puerta"),
		},
		exposed: map[string]map[string]bool{
			"light.kitchen":         {"conversation": true},
			"weather.forecast_casa": {"conversation": true},
			"lock.door":             {"cloud.alexa": true},
		},
		forecasts: map[string]any{
			"service_response": map[string]any{
				"weather.forecast_casa": map[string]any{
					"forecast": []any{
						map[string]any{
							"datetime":                  "2026-11-03T22:00:00+00:00",
							"condition":                 "rainy",
							"temperature":               9.2,
							"precipitation":             0.4,
							"precipitation_probability": 70,
						},
					},
				},
			},
		},
	}
	context, err := newClient(f.start(t)).Context(context.Background())
	if err != nil {
		t.Fatalf("Context: %v", err)
	}
	for _, want := range []string{
		"light.kitchen (La luz de la cocina): off",
		"weather.forecast_casa (Forecast Casa): parcialmente nublado",
		"temperature 9.2",
		"forecast hourly",
		"lluvioso",
		"70 %",
	} {
		if !strings.Contains(context, want) {
			t.Errorf("context does not contain %q:\n%s", want, context)
		}
	}
	if strings.Contains(context, "lock.door") {
		t.Errorf("context contains an unexposed entity:\n%s", context)
	}
	if !slices.Contains(f.calls, "weather/get_forecasts") {
		t.Errorf("calls %v, want weather/get_forecasts", f.calls)
	}
}

func TestSetPower(t *testing.T) {
	f := &fakeHA{}
	c := newClient(f.start(t))
	ctx := context.Background()
	if err := c.SetPower(ctx, "light.kitchen", true); err != nil {
		t.Fatalf("SetPower on: %v", err)
	}
	if err := c.SetPower(ctx, "switch.fan", false); err != nil {
		t.Fatalf("SetPower off: %v", err)
	}
	if !slices.Equal(f.calls, []string{"light/turn_on", "switch/turn_off"}) {
		t.Errorf("calls %v", f.calls)
	}
	if f.bodies[0]["entity_id"] != "light.kitchen" || len(f.bodies[0]) != 1 {
		t.Errorf("body %v, want only the entity_id", f.bodies[0])
	}
}

func TestSetPowerRejectsOtherEntities(t *testing.T) {
	f := &fakeHA{}
	c := newClient(f.start(t))
	for _, id := range []string{"lock.door", "light", "light.", "light.kitchen/../x", "light.Kitchen", "script.turn_on", "light.a,light.b", ""} {
		if err := c.SetPower(context.Background(), id, true); !errors.Is(err, ErrNotAllowed) {
			t.Errorf("SetPower(%q): error %v, want ErrNotAllowed", id, err)
		}
	}
	if len(f.calls) != 0 {
		t.Errorf("calls %v for rejected IDs", f.calls)
	}
}

func TestSetPowerReportsServiceError(t *testing.T) {
	f := &fakeHA{failCall: true}
	err := newClient(f.start(t)).SetPower(context.Background(), "light.kitchen", true)
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error %v, want the HTTP status", err)
	}
}

func TestSetPowerRejectsBadToken(t *testing.T) {
	c := New(Config{URL: (&fakeHA{}).start(t), Token: "wrong", Timeout: time.Second})
	if err := c.SetPower(context.Background(), "light.kitchen", true); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error %v, want ErrUnauthorized", err)
	}
}

func conversation(answerType, speech string) map[string]any {
	return map[string]any{
		"response": map[string]any{
			"response_type": answerType,
			"speech":        map[string]any{"plain": map[string]any{"speech": speech}},
		},
	}
}

func TestConverseUsesTheHomeAssistantAgent(t *testing.T) {
	f := &fakeHA{conversation: conversation("query_answer", "Hace 21 grados y está despejado.")}
	c := New(Config{
		URL:      f.start(t),
		Token:    testToken,
		Timeout:  time.Second,
		Language: "es",
		AgentID:  "conversation.test",
	})
	reply, handled, err := c.Converse(context.Background(), "¿Qué tiempo hace?")
	if err != nil {
		t.Fatalf("Converse: %v", err)
	}
	if !handled || reply != "Hace 21 grados y está despejado." {
		t.Fatalf("reply %q, handled %v", reply, handled)
	}
	if len(f.conversationAt) != 1 {
		t.Fatalf("conversation calls %v", f.conversationAt)
	}
	body := f.conversationAt[0]
	if body["text"] != "¿Qué tiempo hace?" || body["language"] != "es" || body["agent_id"] != "conversation.test" {
		t.Errorf("body %v", body)
	}
}

func TestConverseWithoutLanguageOrAgent(t *testing.T) {
	f := &fakeHA{}
	if _, _, err := newClient(f.start(t)).Converse(context.Background(), "abre la persiana"); err != nil {
		t.Fatalf("Converse: %v", err)
	}
	body := f.conversationAt[0]
	if _, ok := body["language"]; ok {
		t.Errorf("body %v has a language", body)
	}
	if _, ok := body["agent_id"]; ok {
		t.Errorf("body %v has an agent_id", body)
	}
}

func TestConverseDoesNotHandleAnErrorResponse(t *testing.T) {
	f := &fakeHA{conversation: conversation("error", "No te he entendido.")}
	reply, handled, err := newClient(f.start(t)).Converse(context.Background(), "hola")
	if err != nil || handled || reply != "" {
		t.Fatalf("reply %q, handled %v, error %v", reply, handled, err)
	}
}

func TestConverseUsesTheDefaultReplyForAnEmptyAction(t *testing.T) {
	f := &fakeHA{conversation: conversation("action_done", "")}
	reply, handled, err := newClient(f.start(t)).Converse(context.Background(), "abre la persiana")
	if err != nil || !handled || reply != commands.ReplyDone {
		t.Fatalf("reply %q, handled %v, error %v", reply, handled, err)
	}
}

func TestConverseUsesItsOwnTimeout(t *testing.T) {
	f := &fakeHA{converseDelay: 100 * time.Millisecond}
	c := New(Config{URL: f.start(t), Token: testToken, Timeout: time.Second, ConversationTimeout: 10 * time.Millisecond})
	if _, _, err := c.Converse(context.Background(), "abre la persiana"); err == nil {
		t.Fatal("Converse returned no timeout error")
	}
}

func TestConverseReportsServiceError(t *testing.T) {
	f := &fakeHA{failConverse: true}
	_, _, err := newClient(f.start(t)).Converse(context.Background(), "abre la persiana")
	if err == nil || !strings.Contains(err.Error(), "500") {
		t.Fatalf("error %v, want the HTTP status", err)
	}
}

func TestConverseRejectsBadToken(t *testing.T) {
	c := New(Config{URL: (&fakeHA{}).start(t), Token: "wrong", Timeout: time.Second})
	if _, _, err := c.Converse(context.Background(), "abre la persiana"); !errors.Is(err, ErrUnauthorized) {
		t.Fatalf("error %v, want ErrUnauthorized", err)
	}
}
