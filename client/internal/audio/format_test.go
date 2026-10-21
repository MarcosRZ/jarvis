package audio

import (
	"math"
	"testing"
	"time"
)

func TestFrameSize(t *testing.T) {
	if got := SpeechFormat.FrameSize(); got != 2 {
		t.Errorf("speech frame size %d, want 2", got)
	}
	if got := (Format{SampleRate: 48000, Channels: 2}).FrameSize(); got != 4 {
		t.Errorf("stereo frame size %d, want 4", got)
	}
}

func TestBytesFor(t *testing.T) {
	tests := []struct {
		d    time.Duration
		want int
	}{
		{time.Second, 32000},
		{time.Millisecond, 32},
		{100 * time.Microsecond, 2},
		{0, 0},
	}
	for _, tt := range tests {
		if got := SpeechFormat.BytesFor(tt.d); got != tt.want {
			t.Errorf("BytesFor(%v) = %d, want %d", tt.d, got, tt.want)
		}
	}
	if got := (Format{SampleRate: 22050, Channels: 1}).BytesFor(time.Second); got != 44100 {
		t.Errorf("22050 Hz BytesFor(1s) = %d, want 44100", got)
	}
}

func TestDuration(t *testing.T) {
	if got := SpeechFormat.Duration(32000); got != time.Second {
		t.Errorf("Duration(32000) = %v, want 1s", got)
	}
	if got := SpeechFormat.Duration(3); got != 62500*time.Nanosecond {
		t.Errorf("Duration(3) = %v, want one whole frame (62.5µs)", got)
	}
}

func pcm(samples ...int16) []byte {
	b := make([]byte, 2*len(samples))
	for i, s := range samples {
		b[2*i] = byte(s)
		b[2*i+1] = byte(uint16(s) >> 8)
	}
	return b
}

func TestRMSDBFS(t *testing.T) {
	if got := RMSDBFS(pcm(16384, -16384, 16384, -16384)); math.Abs(got-(-6.0206)) > 0.001 {
		t.Errorf("half-scale square = %v dBFS, want -6.02", got)
	}
	if got := RMSDBFS(pcm(16384, 0, 0, 0)); math.Abs(got-(-12.0412)) > 0.001 {
		t.Errorf("one sample in four = %v dBFS, want -12.04", got)
	}
	if got := RMSDBFS(pcm(0, 0)); !math.IsInf(got, -1) {
		t.Errorf("silence = %v dBFS, want -Inf", got)
	}
}

func TestPeakDBFS(t *testing.T) {
	if got := PeakDBFS(pcm(0, -32768, 5)); got != 0 {
		t.Errorf("full scale = %v dBFS, want 0", got)
	}
	if got := PeakDBFS(pcm(16384, -100)); math.Abs(got-(-6.0206)) > 0.001 {
		t.Errorf("half scale = %v dBFS, want -6.02", got)
	}
	if got := PeakDBFS(pcm(0, 0)); !math.IsInf(got, -1) {
		t.Errorf("silence = %v dBFS, want -Inf", got)
	}
	if got := PeakDBFS(nil); !math.IsInf(got, -1) {
		t.Errorf("empty = %v dBFS, want -Inf", got)
	}
}
