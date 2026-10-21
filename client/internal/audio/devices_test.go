package audio

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"testing"
)

func TestLabelNamesMakesDuplicatesUnique(t *testing.T) {
	got := labelNames([]string{"Monitor", "Mic", "Monitor", "Monitor"})
	want := []string{"Monitor", "Mic", "Monitor #2", "Monitor #3"}
	if !slices.Equal(got, want) {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestFindDevice(t *testing.T) {
	devices := []Device{{Label: "Mic"}, {Label: "Monitor"}, {Label: "Monitor #2"}}

	if d, err := findDevice(devices, "", "input"); err != nil || d != nil {
		t.Errorf("empty label = %v, %v; want system default (nil)", d, err)
	}
	if d, err := findDevice(devices, "Monitor #2", "input"); err != nil || d.Label != "Monitor #2" {
		t.Errorf("Monitor #2 = %v, %v", d, err)
	}

	_, err := findDevice(devices, "Speaker", "input")
	if err == nil {
		t.Fatal("unknown device returned no error")
	}
	for _, m := range []string{`"Speaker"`, `"Mic"`, `"Monitor #2"`} {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("error %q does not mention %s", err, m)
		}
	}
}

func TestFindDeviceWithNoDevices(t *testing.T) {
	for _, label := range []string{"", "Mic"} {
		_, err := findDevice(nil, label, "input")
		if err == nil || !strings.Contains(err.Error(), "no input device found") {
			t.Errorf("label %q: error %v, want that there is no input device", label, err)
		}
	}
}

func TestMessagesExplainErrors(t *testing.T) {
	m := &messages{}
	base := errors.New("open default audio device: miniaudio: unknown error")
	if err := m.explain(base); err != base {
		t.Errorf("no messages: %v, want the error unchanged", err)
	}
	for i := range keptMessages + 2 {
		m.add(fmt.Sprintf("message %d", i))
	}
	err := m.explain(base)
	if !errors.Is(err, base) {
		t.Errorf("%v does not wrap the error", err)
	}
	want := "(miniaudio: message 2; message 3; message 4; message 5; message 6)"
	if !strings.HasSuffix(err.Error(), want) {
		t.Errorf("error %q, want the last %d messages %s", err, keptMessages, want)
	}
	if err := m.explain(base); err != base {
		t.Errorf("explained messages are reported again: %v", err)
	}
}
