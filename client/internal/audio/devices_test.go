package audio

import (
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

	if d, err := findDevice(devices, ""); err != nil || d != nil {
		t.Errorf("empty label = %v, %v; want system default (nil)", d, err)
	}
	if d, err := findDevice(devices, "Monitor #2"); err != nil || d.Label != "Monitor #2" {
		t.Errorf("Monitor #2 = %v, %v", d, err)
	}

	_, err := findDevice(devices, "Speaker")
	if err == nil {
		t.Fatal("unknown device returned no error")
	}
	for _, m := range []string{`"Speaker"`, `"Mic"`, `"Monitor #2"`} {
		if !strings.Contains(err.Error(), m) {
			t.Errorf("error %q does not mention %s", err, m)
		}
	}
}
