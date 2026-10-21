package audio

import (
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func TestDefaultBeepIsShortAndNotTooLoud(t *testing.T) {
	pcm := DefaultBeep.PCM()
	if d := BeepFormat.Duration(len(pcm)); d != 140*time.Millisecond {
		t.Errorf("beep is %v, want 140ms", d)
	}
	if peak := PeakDBFS(pcm); peak > DefaultBeep.Level+0.1 || peak < DefaultBeep.Level-1 {
		t.Errorf("peak %.1f dBFS, want about %v", peak, DefaultBeep.Level)
	}
}

func TestBeepNotesStartAndEndSilent(t *testing.T) {
	b := Beep{Notes: []float64{440, 660, 880}, NoteDuration: 50 * time.Millisecond, Fade: 5 * time.Millisecond, Level: -6}
	pcm := b.PCM()
	note := BeepFormat.BytesFor(b.NoteDuration)
	if len(pcm) != 3*note {
		t.Fatalf("%d bytes, want %d", len(pcm), 3*note)
	}
	for n := range 3 {
		for _, i := range []int{n * note, (n+1)*note - BytesPerSample} {
			if s := int16(binary.LittleEndian.Uint16(pcm[i:])); math.Abs(float64(s)) > 1 {
				t.Errorf("note %d: sample at byte %d is %d, want silence (no click)", n, i, s)
			}
		}
	}
}

func TestBeepWithoutFadeDoesNotPanic(t *testing.T) {
	b := Beep{Notes: []float64{1000}, NoteDuration: 10 * time.Millisecond, Level: -20}
	if len(b.PCM()) != BeepFormat.BytesFor(10*time.Millisecond) {
		t.Error("wrong length")
	}
}
