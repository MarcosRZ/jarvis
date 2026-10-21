package commands

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"
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
		{"Dime la hora.", KindTime, ""},
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
		{"Enciende la luz de la cocina.", KindTurnOn, kitchen.ID},
		{"Prende la luz de la cocina", KindTurnOn, kitchen.ID},
		{"Apaga la luz del salón.", KindTurnOff, lounge.ID},
		{"apaga luz del salon", KindTurnOff, lounge.ID},
		{"Enciende el horno.", KindInvalid, ""},
		{"Enciende.", KindInvalid, ""},
		{"Enciende la luz de la cocina; rm -rf /", KindInvalid, ""},
		{"Apaga la luz de la cocina y la del salón", KindInvalid, ""},
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

func TestParseKeepsInvalidArgument(t *testing.T) {
	if cmd := newHandler(t, nil).Parse("Enciende el horno."); cmd.Argument != "horno" {
		t.Errorf("argument %q, want the device phrase without the article", cmd.Argument)
	}
}

func TestNewRejectsBadAllowlist(t *testing.T) {
	tests := map[string][]Device{
		"empty name":      {{ID: "light.a", Name: " "}},
		"empty id":        {{ID: "", Name: "luz"}},
		"duplicate name":  {{ID: "light.a", Name: "luz del salón"}, {ID: "light.b", Name: "Luz del salon"}},
		"name is a verb":  {{ID: "light.a", Name: "enciende"}},
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
	if cmd := h.Parse("Enciende el ventilador"); cmd.Kind != KindTurnOn || cmd.Device != fan {
		t.Errorf("new device: %+v", cmd)
	}
	if cmd := h.Parse("Enciende la luz de la cocina"); cmd.Kind != KindInvalid {
		t.Errorf("removed device still accepted: %+v", cmd)
	}
	if help := h.Execute(context.Background(), Command{Kind: KindHelp}); !strings.Contains(help, "el ventilador") || strings.Contains(help, "cocina") {
		t.Errorf("help %q does not match the new list", help)
	}
}

func TestDeviceRepliesLowercaseALeadingArticle(t *testing.T) {
	devices := &fakeDevices{err: errors.New("unreachable")}
	h := newHandler(t, devices)
	h.SetDevices([]Device{{ID: "light.kitchen", Name: "La luz de la cocina"}, {ID: "switch.ac", Name: "AC"}})
	if got := h.Handle(context.Background(), "enciende la luz de la cocina"); got != "No he podido encender la luz de la cocina." {
		t.Errorf("reply %q", got)
	}
	if got := h.Handle(context.Background(), "apaga AC"); got != "No he podido apagar AC." {
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

func TestHandleRefreshesDevicesFromSource(t *testing.T) {
	clock := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	h, err := New(func() time.Time { return clock }, nil, &fakeDevices{})
	if err != nil {
		t.Fatal(err)
	}
	source := &fakeSource{devices: []Device{kitchen}}
	var reported []error
	h.UseSource(source, 30*time.Second, func(err error) { reported = append(reported, err) })
	ctx := context.Background()

	if got := h.Handle(ctx, "enciende la luz de la cocina"); got != ReplyDone {
		t.Fatalf("first command: %q", got)
	}
	source.devices = []Device{kitchen, lounge}
	clock = clock.Add(10 * time.Second)
	if got := h.Handle(ctx, "apaga la luz del salón"); got != `No conozco el dispositivo "luz del salon".` {
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
}

func TestHandleKeepsDevicesWhenSourceFails(t *testing.T) {
	clock := time.Date(2026, 10, 20, 10, 0, 0, 0, time.Local)
	h, _ := New(func() time.Time { return clock }, nil, &fakeDevices{})
	source := &fakeSource{devices: []Device{kitchen}}
	var reported []error
	h.UseSource(source, time.Second, func(err error) { reported = append(reported, err) })
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
	if got := h.Handle(context.Background(), "Enciende el horno."); got != `No conozco el dispositivo "horno".` {
		t.Errorf("invalid device reply %q", got)
	}
	if got := h.Handle(context.Background(), "Enciende."); got != ReplyMissingDevice {
		t.Errorf("missing device reply %q", got)
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
