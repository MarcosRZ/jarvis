package commands

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/learning"
)

func at(hour, minute int) func() time.Time {
	return func() time.Time { return time.Date(2026, 10, 20, hour, minute, 0, 0, time.Local) }
}

var (
	kitchen = Device{ID: "light.kitchen", Name: "la luz de la cocina"}
	lounge  = Device{ID: "light.lounge", Name: "la luz del salón"}
)

// fakeDevices is a DeviceController that records each call and can fail.
type fakeDevices struct {
	calls []string
	err   error
}

func (f *fakeDevices) SetPower(_ context.Context, id string, on bool) error {
	state := "off"
	if on {
		state = "on"
	}
	f.calls = append(f.calls, id+"="+state)
	return f.err
}

func newHandler(t *testing.T, controller DeviceController) *Handler {
	t.Helper()
	h, err := New(at(22, 15), []Device{kitchen, lounge}, controller)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return h
}

func TestNormalize(t *testing.T) {
	tests := map[string]string{
		"¿Qué hora es?":          "que hora es",
		"  ¡Qué   HORAS!  ":      "que horas",
		"Qué hora es, por favor": "que hora es por favor",
		"Pingüino Ñandú":         "pinguino nandu",
	}
	for in, want := range tests {
		if got := normalize(in); got != want {
			t.Errorf("normalize(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParse(t *testing.T) {
	h := newHandler(t, nil)
	tests := []struct {
		text   string
		kind   Kind
		device string
	}{
		{"¿Qué hora es?", KindTime, ""},
		{"Dime la hora, por favor.", KindTime, ""},
		{"¡Qué horas!", KindTime, ""},
		{"que ahora es", KindTime, ""},
		{"¿Sabes qué hora es?", KindTime, ""},
		{"¿Qué hora es ya?", KindTime, ""},
		{"¿Qué horas son?", KindTime, ""},
		{"Repite lo que he dicho.", KindRepeat, ""},
		{"Repite.", KindRepeat, ""},
		{"Cancela.", KindCancel, ""},
		{"¡Olvídalo!", KindCancel, ""},
		{"¿Qué puedes hacer?", KindHelp, ""},
		{"Ayuda.", KindHelp, ""},
		{"Mal.", KindPenalty, ""},
		{"Eso está mal.", KindPenalty, ""},
		{"Te has equivocado.", KindPenalty, ""},
		{"No es eso.", KindPenalty, ""},
		{"Enciende la luz de la cocina.", KindUnknown, ""},
		{"Pon la luz de la cocina", KindUnknown, ""},
		{"Apaga la luz del salón.", KindUnknown, ""},
		{"Enciende el horno.", KindUnknown, ""},
		{"Enciende.", KindUnknown, ""},
		{"por favor", KindUnknown, ""},
		{"Hola, ¿cómo estás?", KindUnknown, ""},
		{"hora", KindUnknown, ""},
		{"", KindUnknown, ""},
	}
	for _, tt := range tests {
		cmd := h.Parse(tt.text)
		if cmd.Kind != tt.kind || cmd.Device.ID != tt.device {
			t.Errorf("Parse(%q) = %+v, want kind %q device %q", tt.text, cmd, tt.kind, tt.device)
		}
	}
}

func TestNewRejectsBadAllowlist(t *testing.T) {
	tests := map[string][]Device{
		"empty name":      {{ID: "light.a", Name: " "}},
		"empty id":        {{ID: "", Name: "luz"}},
		"duplicate name":  {{ID: "light.a", Name: "luz del salón"}, {ID: "light.b", Name: "Luz del salon"}},
		"name with comma": {{ID: "light.a", Name: "luz, cocina"}},
	}
	for name, devices := range tests {
		if _, err := New(time.Now, devices, nil); !errors.Is(err, ErrBadDevice) {
			t.Errorf("%s: error %v, want ErrBadDevice", name, err)
		}
	}
}

func TestSetDevicesReplacesAllowlistAndSkipsBadDevices(t *testing.T) {
	h := newHandler(t, nil)
	fan := Device{ID: "switch.fan", Name: "El ventilador"}
	problems := h.SetDevices([]Device{
		fan,
		{ID: "light.entrance", Name: "Entrance Color + White Lights"},
		{ID: "light.copy", Name: "el Ventilador"},
		{ID: "", Name: "la lámpara"},
	})
	if len(problems) != 3 {
		t.Fatalf("problems %v, want 3", problems)
	}
	for _, p := range problems {
		if !errors.Is(p, ErrBadDevice) {
			t.Errorf("problem %v is not ErrBadDevice", p)
		}
	}
	if named, ok := h.namedDevice("enciende el ventilador"); !ok || named != fan {
		t.Errorf("new device: %+v, %v", named, ok)
	}
	if named, ok := h.namedDevice("enciende la luz de la cocina"); ok {
		t.Errorf("removed device still named: %+v", named)
	}
	if help := h.Execute(context.Background(), Command{Kind: KindHelp}); !strings.Contains(help, "el ventilador") || strings.Contains(help, "cocina") {
		t.Errorf("help %q does not match the new list", help)
	}
}

func TestDeviceRepliesLowercaseALeadingArticle(t *testing.T) {
	devices := &fakeDevices{err: errors.New("unreachable")}
	h := newHandler(t, devices)
	h.SetDevices([]Device{{ID: "light.kitchen", Name: "La luz de la cocina"}, {ID: "switch.ac", Name: "AC"}})
	if got := h.Execute(context.Background(), Command{Kind: KindTurnOn, Device: Device{ID: "light.kitchen", Name: "La luz de la cocina"}}); got != "No he podido encender la luz de la cocina." {
		t.Errorf("reply %q", got)
	}
	if got := h.Execute(context.Background(), Command{Kind: KindTurnOff, Device: Device{ID: "switch.ac", Name: "AC"}}); got != "No he podido apagar AC." {
		t.Errorf("reply %q, want the name unchanged when it has no article", got)
	}
}

// fakeSource is a DeviceSource with a list that the test can change.
type fakeSource struct {
	devices []Device
	err     error
	fetches int
}

func (s *fakeSource) Devices(context.Context) ([]Device, error) {
	s.fetches++
	return s.devices, s.err
}

func TestNamedDevice(t *testing.T) {
	h := newHandler(t, nil)
	tests := map[string]string{
		"enciende la luz de la cocina":   kitchen.ID,
		"la luz del salón":               lounge.ID,
		"quiero apagar la luz del salón": lounge.ID,
		"necesito luz en la cocina":      kitchen.ID,
		"enciende la luz":                "",
		"enciende el ventilador":         "",
		"hace frío":                      "",
	}
	for text, want := range tests {
		got, ok := h.namedDevice(text)
		if want == "" {
			if ok {
				t.Errorf("namedDevice(%q) = %s, want none", text, got.ID)
			}
			continue
		}
		if !ok || got.ID != want {
			t.Errorf("namedDevice(%q) = %s, %v, want %s", text, got.ID, ok, want)
		}
	}
}

func TestHandleRefreshesDevicesFromSource(t *testing.T) {
	clock := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	devices := &fakeDevices{}
	h, err := New(func() time.Time { return clock }, nil, devices)
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{devices: []Device{kitchen}}
	var reported []error
	h.UseSource(source, 30*time.Second, func(err error) { reported = append(reported, err) })
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()

	if got := h.Handle(ctx, "enciende la luz de la cocina"); got != ReplyDone {
		t.Fatalf("first command: %q", got)
	}
	source.devices = []Device{kitchen, lounge}
	interp.commands = []Command{{Kind: KindTurnOff, Device: lounge}}
	clock = clock.Add(10 * time.Second)
	if got := h.Handle(ctx, "apaga la luz del salón"); got != ReplyUnknown {
		t.Errorf("within the refresh time the new device must not be known yet: %q", got)
	}
	clock = clock.Add(30 * time.Second)
	if got := h.Handle(ctx, "apaga la luz del salón"); got != ReplyDone {
		t.Errorf("after the refresh time: %q", got)
	}
	if source.fetches != 2 {
		t.Errorf("%d fetches, want 2", source.fetches)
	}
	if len(reported) != 0 {
		t.Errorf("reported %v", reported)
	}
	if want := []string{"light.kitchen=on", "light.lounge=off"}; !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
}

func TestHandleKeepsDevicesWhenSourceFails(t *testing.T) {
	clock := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	h, _ := New(func() time.Time { return clock }, nil, &fakeDevices{})
	source := &fakeSource{devices: []Device{kitchen}}
	var reported []error
	h.UseSource(source, time.Second, func(err error) { reported = append(reported, err) })
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()
	h.Handle(ctx, "hola")

	down := errors.New("Home Assistant down")
	source.err, source.devices = down, nil
	clock = clock.Add(time.Minute)
	if got := h.Handle(ctx, "enciende la luz de la cocina"); got != ReplyDone {
		t.Errorf("reply %q, want the last known list to work", got)
	}
	if len(reported) != 1 || !errors.Is(reported[0], down) {
		t.Errorf("reported %v, want the source error", reported)
	}
}

func TestHandleReportsBadDevicesOncePerList(t *testing.T) {
	clock := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	h, _ := New(func() time.Time { return clock }, nil, &fakeDevices{})
	bad := Device{ID: "light.entrance", Name: "Entrance Color + White Lights"}
	source := &fakeSource{devices: []Device{kitchen, bad}}
	var reported []error
	h.UseSource(source, time.Second, func(err error) { reported = append(reported, err) })
	for range 3 {
		h.Handle(context.Background(), "hola")
		clock = clock.Add(time.Minute)
	}
	if len(reported) != 1 || !errors.Is(reported[0], ErrBadDevice) {
		t.Errorf("reported %v, want one ErrBadDevice", reported)
	}
	if source.fetches != 3 {
		t.Errorf("%d fetches, want 3", source.fetches)
	}
}

func TestExecuteFixedReplies(t *testing.T) {
	h := newHandler(t, nil)
	ctx := context.Background()
	if got := h.Execute(ctx, Command{Kind: KindTime}); got != "Son las 10 y cuarto." {
		t.Errorf("time: %q", got)
	}
	if got := h.Execute(ctx, Command{Kind: KindCancel}); got != ReplyCancelled {
		t.Errorf("cancel: %q", got)
	}
	if got := h.Execute(ctx, Command{Kind: KindUnknown}); got != ReplyUnknown {
		t.Errorf("unknown: %q", got)
	}
	help := h.Execute(ctx, Command{Kind: KindHelp})
	for _, part := range []string{"la hora", "repetir", "cancelar", "luz de la cocina", "luz del salón"} {
		if !strings.Contains(help, part) {
			t.Errorf("help %q does not mention %q", help, part)
		}
	}
}

func TestExecuteHelpWithoutDevices(t *testing.T) {
	h, err := New(time.Now, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if help := h.Execute(context.Background(), Command{Kind: KindHelp}); strings.Contains(help, "encender") {
		t.Errorf("help %q offers devices, but none are configured", help)
	}
}

func TestExecuteHelpLeavesOutUnavailableDevices(t *testing.T) {
	h, err := New(time.Now, []Device{kitchen, {ID: "light.led", Name: "El LED", Unavailable: true}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if help := h.Execute(context.Background(), Command{Kind: KindHelp}); !strings.HasSuffix(help, " También puedo encender y apagar la luz de la cocina.") {
		t.Errorf("help %q, want only the available device", help)
	}
	h.SetDevices([]Device{{ID: "light.led", Name: "El LED", Unavailable: true}})
	if help := h.Execute(context.Background(), Command{Kind: KindHelp}); strings.Contains(help, "encender") {
		t.Errorf("help %q offers devices, but none is available", help)
	}
}

func TestExecuteDevice(t *testing.T) {
	ctx := context.Background()
	devices := &fakeDevices{}
	h := newHandler(t, devices)

	if got := h.Execute(ctx, Command{Kind: KindTurnOn, Device: kitchen}); got != ReplyDone {
		t.Errorf("turn on: %q", got)
	}
	if got := h.Execute(ctx, Command{Kind: KindTurnOff, Device: lounge}); got != ReplyDone {
		t.Errorf("turn off: %q", got)
	}
	if !slices.Equal(devices.calls, []string{"light.kitchen=on", "light.lounge=off"}) {
		t.Errorf("calls %v", devices.calls)
	}

	devices.err = errors.New("timeout")
	if got := h.Execute(ctx, Command{Kind: KindTurnOn, Device: kitchen}); got != "No he podido encender la luz de la cocina." {
		t.Errorf("failure: %q", got)
	}
}

func TestExecuteWithoutController(t *testing.T) {
	h := newHandler(t, nil)
	if got := h.Execute(context.Background(), Command{Kind: KindTurnOn, Device: kitchen}); got != ReplyNoDevices {
		t.Errorf("reply %q, want %q", got, ReplyNoDevices)
	}
}

func TestInvalidAndUnknownCommandsNeverReachDevices(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	for _, text := range []string{"Enciende el horno.", "Enciende.", "Enciende la luz de la cocina; rm -rf /", "Abre la puerta.", "hola"} {
		reply := h.Handle(context.Background(), text)
		if reply == "" {
			t.Errorf("no reply for %q", text)
		}
	}
	if len(devices.calls) != 0 {
		t.Fatalf("device calls %v for invalid or unknown commands", devices.calls)
	}
	if got := h.Handle(context.Background(), "Enciende el horno."); got != ReplyUnknown {
		t.Errorf("unknown device reply %q, want %q", got, ReplyUnknown)
	}
}

func TestHandleRepeatsPreviousText(t *testing.T) {
	ctx := context.Background()
	h := newHandler(t, nil)
	if got := h.Handle(ctx, "Repite lo que he dicho."); got != ReplyNothingToRepeat {
		t.Errorf("first repeat: %q", got)
	}
	h.Handle(ctx, "¿Qué hora es?")
	for range 2 {
		if got := h.Handle(ctx, "Repite lo que he dicho."); got != "Has dicho: ¿Qué hora es?" {
			t.Errorf("repeat: %q", got)
		}
	}
}

func TestParseAll(t *testing.T) {
	h := newHandler(t, nil)
	tests := []struct {
		text string
		want []Command
	}{
		{"Enciende la luz de la cocina.", []Command{{Kind: KindUnknown}}},
		{"Enciende la luz de la cocina y apaga la luz del salón.", []Command{{Kind: KindUnknown}}},
		{"¿Qué hora es y qué puedes hacer?", []Command{
			{Kind: KindTime},
			{Kind: KindHelp},
		}},
		{"Apaga la luz del salón y dime qué hora es.", []Command{{Kind: KindUnknown}}},
		{"apaga la luz del salon y dime que hora es", []Command{{Kind: KindUnknown}}},
		{"", []Command{{Kind: KindUnknown}}},
	}
	for _, tt := range tests {
		got := h.ParseAll(tt.text)
		if len(got) != len(tt.want) {
			t.Errorf("ParseAll(%q) = %+v, want %+v", tt.text, got, tt.want)
			continue
		}
		for i := range got {
			if got[i].Kind != tt.want[i].Kind || got[i].Device.ID != tt.want[i].Device.ID {
				t.Errorf("ParseAll(%q)[%d] = %+v, want %+v", tt.text, i, got[i], tt.want[i])
			}
		}
	}
}

func TestHandleUsesTheModelForADeviceSentence(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)
	if got := h.Handle(context.Background(), "enciende la luz de la cocina"); got != ReplyDone {
		t.Errorf("reply %q", got)
	}
	if !slices.Equal(interp.texts, []string{"enciende la luz de la cocina"}) {
		t.Errorf("interpreter texts %v", interp.texts)
	}
	if !slices.Equal(devices.calls, []string{"light.kitchen=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestSpokenTime(t *testing.T) {
	tests := []struct {
		hour, minute int
		want         string
	}{
		{13, 0, "Es la una en punto."},
		{1, 5, "Es la una y 5."},
		{0, 30, "Son las 12 y media."},
		{12, 15, "Son las 12 y cuarto."},
		{9, 45, "Son las 9 y 45."},
		{23, 59, "Son las 11 y 59."},
	}
	for _, tt := range tests {
		if got := spokenTime(at(tt.hour, tt.minute)()); got != tt.want {
			t.Errorf("%02d:%02d = %q, want %q", tt.hour, tt.minute, got, tt.want)
		}
	}
}

// fakeInterpreter records each text and the devices it received, and returns
// the configured commands.
type fakeInterpreter struct {
	texts    []string
	devices  [][]Device
	contexts []string
	commands []Command
	reply    string
	err      error
}

func (f *fakeInterpreter) Interpret(_ context.Context, text string, devices []Device) ([]Command, error) {
	f.texts = append(f.texts, text)
	f.devices = append(f.devices, devices)
	return f.commands, f.err
}

func (f *fakeInterpreter) InterpretContext(_ context.Context, text string, devices []Device, homeContext string) (Interpretation, error) {
	f.texts = append(f.texts, text)
	f.devices = append(f.devices, devices)
	f.contexts = append(f.contexts, homeContext)
	return Interpretation{Commands: f.commands, Reply: f.reply}, f.err
}

// fakeContext returns the configured read-only Home Assistant context.
type fakeContext struct {
	text string
	err  error
}

func (f *fakeContext) Context(context.Context) (string, error) {
	return f.text, f.err
}

// fakeConversation records each text and returns the configured answer.
type fakeConversation struct {
	texts   []string
	reply   string
	handled bool
	err     error
}

func (f *fakeConversation) Converse(_ context.Context, text string) (string, bool, error) {
	f.texts = append(f.texts, text)
	return f.reply, f.handled, f.err
}

func TestHandleUsesConversationForUnresolvedSentences(t *testing.T) {
	ctx := context.Background()
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	conversation := &fakeConversation{reply: "Hecho por Home Assistant.", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(ctx, "abre la persiana"); got != conversation.reply {
		t.Errorf("unknown reply %q", got)
	}
	if got := h.Handle(ctx, "enciende el horno"); got != conversation.reply {
		t.Errorf("invalid reply %q", got)
	}
	if got := h.Handle(ctx, "¿Qué tiempo hace?"); got != conversation.reply {
		t.Errorf("question reply %q", got)
	}
	if want := []string{"abre la persiana", "enciende el horno", "¿Qué tiempo hace?"}; !slices.Equal(conversation.texts, want) {
		t.Errorf("conversation texts %v, want %v", conversation.texts, want)
	}

	if got := h.Handle(ctx, "¿Qué hora es?"); got != spokenTime(h.now()) {
		t.Errorf("fixed command reply %q", got)
	}
	if len(conversation.texts) != 3 {
		t.Errorf("conversation was asked for a fixed command: %v", conversation.texts)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v, want none", devices.calls)
	}
}

func TestHandleKeepsTheLocalReplyWhenConversationDoesNotUnderstand(t *testing.T) {
	ctx := context.Background()
	h := newHandler(t, nil)
	conversation := &fakeConversation{}
	h.UseConversation(conversation)

	if got := h.Handle(ctx, "abre la persiana"); got != ReplyUnknown {
		t.Errorf("unknown reply %q, want %q", got, ReplyUnknown)
	}
	if got := h.Handle(ctx, "enciende el horno"); got != ReplyUnknown {
		t.Errorf("device reply %q, want %q", got, ReplyUnknown)
	}
	if len(conversation.texts) != 2 {
		t.Errorf("conversation texts %v", conversation.texts)
	}
}

func TestHandleFallsBackWhenConversationFails(t *testing.T) {
	h := newHandler(t, nil)
	h.UseConversation(&fakeConversation{err: errors.New("Home Assistant down")})

	if got := h.Handle(context.Background(), "abre la persiana"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
}

func TestHandleUsesConversationWhenTheInterpreterFails(t *testing.T) {
	ctx := context.Background()
	h := newHandler(t, nil)
	interp := &fakeInterpreter{err: errors.New("language model down")}
	h.UseInterpreter(interp, InterpretFallback)
	h.UseInference(FallbackModelFirst, ActionGuard)
	conversation := &fakeConversation{reply: "Hecho por Home Assistant.", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(ctx, "Quiero apagar la luz del salón."); got != conversation.reply {
		t.Errorf("reply %q", got)
	}
	if len(interp.texts) != 1 {
		t.Errorf("interpreter texts %v", interp.texts)
	}
	if !slices.Equal(conversation.texts, []string{"Quiero apagar la luz del salón."}) {
		t.Errorf("conversation texts %v", conversation.texts)
	}
}

func TestHandleUsesConversationBeforeTheInterpreterForUnresolvedCommands(t *testing.T) {
	ctx := context.Background()
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	conversation := &fakeConversation{reply: "He puesto el aire a 22 grados.", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(ctx, "pon el aire a 22 grados"); got != conversation.reply {
		t.Errorf("reply %q", got)
	}
	if len(interp.texts) != 0 {
		t.Errorf("interpreter was asked: %v", interp.texts)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleRunsTheLocalStateBeforeConversation(t *testing.T) {
	ctx := context.Background()
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseRoom("salón")
	conversation := &fakeConversation{reply: "Hecho por Home Assistant.", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(ctx, "no veo nada"); got != ReplyDone {
		t.Errorf("reply %q", got)
	}
	if len(conversation.texts) != 0 {
		t.Errorf("conversation was asked for a local state: %v", conversation.texts)
	}
	if want := []string{"light.lounge=on"}; !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
}

func TestHandleRunsFixedClauses(t *testing.T) {
	h := newHandler(t, nil)
	got := h.Handle(context.Background(), "dime la hora y qué puedes hacer")
	want := spokenTime(h.now()) + " " + h.Execute(context.Background(), Command{Kind: KindHelp})
	if got != want {
		t.Errorf("reply %q, want %q", got, want)
	}
}

func TestHandleUsesInterpreterOnUnresolvedSentences(t *testing.T) {
	ctx := context.Background()
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOff, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(ctx, "Quiero apagar la luz del salón."); got != ReplyDone {
		t.Errorf("interpreted command: %q", got)
	}
	if !slices.Equal(interp.texts, []string{"Quiero apagar la luz del salón."}) {
		t.Errorf("interpreter texts %v", interp.texts)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=off"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleGivesContextToModelAndSpeaksReply(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseContext(&fakeContext{text: `weather.forecast_casa: partlycloudy
  forecast hourly:
  - {"condition":"rain","precipitation_probability":70}`})
	interp := &fakeInterpreter{reply: "Sí, hay un 70 % de probabilidad de lluvia."}
	h.UseInterpreter(interp, InterpretFallback)

	for _, text := range []string{"¿Va a llover?", "Va a llover."} {
		if got := h.Handle(context.Background(), text); got != interp.reply {
			t.Errorf("Handle(%q) = %q, want %q", text, got, interp.reply)
		}
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v, want none", devices.calls)
	}
	if len(interp.contexts) != 2 || !strings.Contains(interp.contexts[0], "precipitation_probability") {
		t.Errorf("interpreter contexts %v", interp.contexts)
	}
}

func TestHandleRejectsReplyWithoutContext(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{reply: "Sí, va a llover."}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "¿Va a llover?"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v, want none", devices.calls)
	}
}

func TestHandleRejectsEmptyOrEchoedReply(t *testing.T) {
	for _, reply := range []string{"...", "¿Va a llover?", "Va a llover."} {
		devices := &fakeDevices{}
		h := newHandler(t, devices)
		h.UseContext(&fakeContext{text: "weather.forecast_casa: lluvioso"})
		h.UseInterpreter(&fakeInterpreter{reply: reply}, InterpretFallback)
		if got := h.Handle(context.Background(), "¿Va a llover?"); got != ReplyUnknown {
			t.Errorf("reply %q: got %q, want %q", reply, got, ReplyUnknown)
		}
	}
}

func TestHandleDoesNotSpeakReplyForDeviceQuestion(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseContext(&fakeContext{text: "light.lounge: off"})
	interp := &fakeInterpreter{reply: "Sí."}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "¿Enciendes la luz del salón?"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v, want none", devices.calls)
	}
}

func TestHandleRunsAnInferredAction(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOff, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "hay demasiada luz aquí"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=off"}) {
		t.Errorf("device calls %v", devices.calls)
	}
	// The model gets every available device, so it can reason about the room.
	if len(interp.devices) != 1 || len(interp.devices[0]) != 2 {
		t.Fatalf("interpreter devices %v, want the two allowlisted devices", interp.devices)
	}
}

func TestHandleLearnsAStateAndReusesIt(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()

	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if len(interp.texts) != 1 {
		t.Errorf("model asked %d times, want 1", len(interp.texts))
	}
	if want := []string{"light.lounge=on", "light.lounge=on"}; !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
	e, ok := store.Lookup(learningKey("hay poca claridad"))
	if !ok || e.Score != 2 || e.Successes != 2 {
		t.Errorf("entry %+v, %v", e, ok)
	}
}

func TestHandlePenalizesAndReplacesTheLearnedAction(t *testing.T) {
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, time.Local)
	devices := &fakeDevices{}
	h, err := New(func() time.Time { return now }, []Device{kitchen, lounge}, devices)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()

	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(ctx, "mal"); got != ReplyPenalized {
		t.Fatalf("reply %q", got)
	}
	e, ok := store.Lookup(learningKey("hay poca claridad"))
	if !ok || e.Score != -1 || e.Successes != 0 || e.Failures != 1 {
		t.Fatalf("entry %+v, %v", e, ok)
	}

	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyUnknown {
		t.Fatalf("reply %q, want the punished action to be dropped", got)
	}
	if len(interp.texts) != 2 {
		t.Errorf("model asked %d times, want 2", len(interp.texts))
	}

	interp.commands = []Command{{Kind: KindTurnOff, Device: lounge}}
	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	e, ok = store.Lookup(learningKey("hay poca claridad"))
	if !ok || e.Kind != "turn-off" || e.Score != 1 || e.Successes != 1 || e.Failures != 0 {
		t.Fatalf("entry %+v, %v", e, ok)
	}
	if got := h.Handle(ctx, "hay poca claridad"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if len(interp.texts) != 3 {
		t.Errorf("model asked %d times, want 3", len(interp.texts))
	}
	want := []string{"light.lounge=on", "light.lounge=off", "light.lounge=off"}
	if !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
}

func TestHandlePenalizesTheDeterministicState(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseRoom("salón")
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	ctx := context.Background()

	if got := h.Handle(ctx, "no veo nada"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(ctx, "mal"); got != ReplyPenalized {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(ctx, "no veo nada"); got != ReplyUnknown {
		t.Fatalf("reply %q, want the punished action to be dropped", got)
	}
	if want := []string{"light.lounge=on"}; !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
}

func TestPenaltyExpiresAndNeedsAnAction(t *testing.T) {
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, time.Local)
	devices := &fakeDevices{}
	h, err := New(func() time.Time { return now }, []Device{kitchen, lounge}, devices)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if got := h.Handle(context.Background(), "mal"); got != ReplyNothingToCorrect {
		t.Fatalf("reply %q", got)
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	h.UseRoom("salón")
	if got := h.Handle(context.Background(), "no veo nada"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	now = now.Add(2 * time.Minute)
	if got := h.Handle(context.Background(), "mal"); got != ReplyNothingToCorrect {
		t.Fatalf("reply %q", got)
	}
	e, ok := store.Lookup(learningKey("no veo nada"))
	if !ok || e.Score != 1 || e.Failures != 0 {
		t.Errorf("entry %+v, %v", e, ok)
	}
}

func TestHandleRunsCommonLightStatesWithoutTheModel(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseRoom("salón")
	interp := &fakeInterpreter{}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()

	if got := h.Handle(ctx, "no veo nada"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(ctx, "hay demasiada luz"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	want := []string{"light.lounge=on", "light.lounge=off"}
	if !slices.Equal(devices.calls, want) {
		t.Errorf("device calls %v, want %v", devices.calls, want)
	}
	if len(interp.texts) != 0 {
		t.Errorf("model was asked: %v", interp.texts)
	}
}

func TestCommonLightStateDoesNotGuessBetweenRooms(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	if got := h.Handle(context.Background(), "no veo nada"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleLogsTheAction(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	var logs bytes.Buffer
	h.UseMatching(DefaultMatching, slog.New(slog.NewTextHandler(&logs, nil)), slog.LevelInfo)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOff, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "hay demasiada luz aquí"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	for _, want := range []string{"msg=action", "source=reasoned", "kind=turn-off", "device=light.lounge", `name="la luz del salón"`, "ok=true"} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs have no %q:\n%s", want, logs.String())
		}
	}

}

func TestHandleRunsEveryInferredAction(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{
		{Kind: KindTurnOff, Device: lounge},
		{Kind: KindTurnOn, Device: kitchen},
	}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "hay demasiada luz aquí"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=off", "light.kitchen=on"}) {
		t.Errorf("device calls %v, want both inferred actions", devices.calls)
	}
}

func TestHandleInfersAnyUnresolvedSentenceNotInMemory(t *testing.T) {
	for _, text := range []string{"hay mucha luz", "necesito más luz"} {
		t.Run(text, func(t *testing.T) {
			devices := &fakeDevices{}
			h := newHandler(t, devices)
			interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
			h.UseInterpreter(interp, InterpretFallback)

			if got := h.Handle(context.Background(), text); got != ReplyDone {
				t.Errorf("reply %q, want %q", got, ReplyDone)
			}
			if !slices.Equal(interp.texts, []string{text}) {
				t.Errorf("interpreter texts %v", interp.texts)
			}
			if !slices.Equal(devices.calls, []string{"light.lounge=on"}) {
				t.Errorf("device calls %v", devices.calls)
			}
		})
	}
}

func TestHandleKeepsUnknownWhenInferenceFindsNothing(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "enciende el ventilador"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(interp.texts) != 1 {
		t.Errorf("interpreter texts %v, want one attempt", interp.texts)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleInfersOnlyWithDevices(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "hay mucha luz"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(interp.texts) != 0 {
		t.Errorf("interpreter called without devices: %v", interp.texts)
	}
}

func TestHandleUsesInferenceWhenConversationDoesNotUnderstand(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	conversation := &fakeConversation{handled: false}
	h.UseConversation(conversation)

	if got := h.Handle(context.Background(), "hay mucha luz"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(conversation.texts, []string{"hay mucha luz"}) {
		t.Errorf("conversation texts %v", conversation.texts)
	}
	if !slices.Equal(interp.texts, []string{"hay mucha luz"}) {
		t.Errorf("interpreter texts %v", interp.texts)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleUsesLearnedMemoryForAnyUnresolvedSentence(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	if _, err := store.Learn(learningKey("pon la tele"), lounge.ID, lounge.Name, string(KindTurnOn), time.Now()); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOff, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)
	conversation := &fakeConversation{reply: "otra cosa", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(context.Background(), "pon la tele"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(interp.texts) != 0 {
		t.Errorf("interpreter called for a learned sentence: %v", interp.texts)
	}
	if len(conversation.texts) != 0 {
		t.Errorf("conversation called for a learned sentence: %v", conversation.texts)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleGuardRejectsUnsupportedInference(t *testing.T) {
	for _, text := range []string{"va a llover", "enciende la tele"} {
		t.Run(text, func(t *testing.T) {
			devices := &fakeDevices{}
			h := newHandler(t, devices)
			interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
			h.UseInterpreter(interp, InterpretFallback)

			if got := h.Handle(context.Background(), text); got == ReplyDone {
				t.Errorf("reply %q, want a rejection", got)
			}
			if !slices.Equal(interp.texts, []string{text}) {
				t.Errorf("interpreter texts %v", interp.texts)
			}
			if len(devices.calls) != 0 {
				t.Errorf("device calls %v", devices.calls)
			}
		})
	}
}

func TestHandleGuardRejectsADeviceThatCannotFixTheState(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "hace frío"); got == ReplyDone {
		t.Errorf("reply %q, want a rejection", got)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleGuardRejectsAQuestionAboutAState(t *testing.T) {
	air := Device{ID: "switch.air", Name: "el aire acondicionado"}
	devices := &fakeDevices{}
	h, err := New(at(22, 15), []Device{air}, devices)
	if err != nil {
		t.Fatal(err)
	}
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: air}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "¿hace frío?"); got == ReplyDone {
		t.Errorf("reply %q, want a rejection", got)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
	if got := h.Handle(context.Background(), "hace frío"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(devices.calls, []string{"switch.air=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleActionSafetyLiteralRunsAnyInference(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseInference(FallbackHAFirst, ActionLiteral)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "va a llover"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleActionSafetyStatesRejectsACommand(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseInference(FallbackHAFirst, ActionStates)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "enciende el ventilador"); got != ReplyUnknown {
		t.Errorf("reply %q, want a rejection", got)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleFallbackModelFirstUsesTheModelBeforeConversation(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseInference(FallbackModelFirst, ActionGuard)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	conversation := &fakeConversation{reply: "respuesta de Home Assistant", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(context.Background(), "hay mucha luz"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if !slices.Equal(devices.calls, []string{"light.lounge=on"}) {
		t.Errorf("device calls %v", devices.calls)
	}
	if len(conversation.texts) != 0 {
		t.Errorf("conversation was asked before the model: %v", conversation.texts)
	}
}

func TestHandleFallbackModelFirstUsesConversationWhenTheModelIsRejected(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	h.UseInference(FallbackModelFirst, ActionGuard)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: lounge}}}
	h.UseInterpreter(interp, InterpretFallback)
	conversation := &fakeConversation{reply: "respuesta de Home Assistant", handled: true}
	h.UseConversation(conversation)

	if got := h.Handle(context.Background(), "va a llover"); got != conversation.reply {
		t.Errorf("reply %q, want %q", got, conversation.reply)
	}
	if !slices.Equal(interp.texts, []string{"va a llover"}) {
		t.Errorf("interpreter texts %v", interp.texts)
	}
	if !slices.Equal(conversation.texts, []string{"va a llover"}) {
		t.Errorf("conversation texts %v", conversation.texts)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v", devices.calls)
	}
}

func TestHandleGuardRejectsTheWrongNamedDevice(t *testing.T) {
	devices := &fakeDevices{}
	h := newHandler(t, devices)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTurnOn, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "quiero apagar la luz del salón"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(devices.calls) != 0 {
		t.Errorf("device calls %v for the wrong device", devices.calls)
	}
	if len(interp.texts) != 1 {
		t.Errorf("interpreter texts %v", interp.texts)
	}
}

func TestHandleDropsInterpretedDeviceOutsideAllowlist(t *testing.T) {
	h := newHandler(t, nil)
	interp := &fakeInterpreter{commands: []Command{{
		Kind:   KindTurnOn,
		Device: Device{ID: "light.hall", Name: "la luz del pasillo"},
	}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "enciende el pasillo"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
}

func TestHandleDropsFixedCommandFromInterpreter(t *testing.T) {
	h := newHandler(t, nil)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindTime}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "quiero apagar la luz del salón"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(interp.texts) != 1 {
		t.Errorf("interpreter texts %v", interp.texts)
	}
}

func TestHandleKeepsDeterministicCommandWhenInterpreterFails(t *testing.T) {
	h := newHandler(t, nil)
	interp := &fakeInterpreter{err: errors.New("language model down")}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "Quiero apagar la luz del salón."); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(interp.texts) != 1 {
		t.Errorf("interpreter texts %v", interp.texts)
	}
}

func TestHandleIgnoresDeviceOfFixedInterpretedCommand(t *testing.T) {
	h := newHandler(t, nil)
	interp := &fakeInterpreter{commands: []Command{{Kind: KindRepeat, Device: kitchen}}}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "repite"); got != ReplyNothingToRepeat {
		t.Errorf("reply %q", got)
	}
}
