package audio

import (
	"context"
	"encoding/binary"
	"math"
	"testing"
	"time"
)

func sinePCM(rate int, d time.Duration, tones map[float64]float64) []byte {
	n := int(float64(rate) * d.Seconds())
	pcm := make([]byte, n*BytesPerSample)
	for i := range n {
		var v float64
		for freq, amplitude := range tones {
			v += amplitude * math.Sin(2*math.Pi*freq*float64(i)/float64(rate))
		}
		binary.LittleEndian.PutUint16(pcm[i*2:], uint16(int16(math.Round(v))))
	}
	return pcm
}

func toneAmplitude(pcm []byte, freq, rate float64) float64 {
	n := len(pcm) / BytesPerSample
	var re, im float64
	for i := range n {
		v := float64(int16(binary.LittleEndian.Uint16(pcm[i*BytesPerSample:])))
		phase := 2 * math.Pi * freq * float64(i) / rate
		re += v * math.Cos(phase)
		im += v * math.Sin(phase)
	}
	return math.Hypot(re, im) / float64(n)
}

func TestHighPassAttenuatesRumble(t *testing.T) {
	const rate = 16000
	const rumble = 50.0
	const voice = 1000.0
	pcm := sinePCM(rate, time.Second, map[float64]float64{rumble: 8000, voice: 8000})
	filtered := NewHighPass(&memSource{pcm: pcm}, 120, rate)
	out := make([]byte, len(pcm))
	if err := ReadFull(context.Background(), filtered, out); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	rumbleBefore := toneAmplitude(pcm, rumble, rate)
	rumbleAfter := toneAmplitude(out, rumble, rate)
	voiceBefore := toneAmplitude(pcm, voice, rate)
	voiceAfter := toneAmplitude(out, voice, rate)

	attenuation := 20 * math.Log10(rumbleBefore/rumbleAfter)
	if attenuation < 20 {
		t.Errorf("50 Hz attenuated %.1f dB, want at least 20", attenuation)
	}
	change := 20 * math.Log10(voiceAfter/voiceBefore)
	if math.Abs(change) > 1 {
		t.Errorf("1 kHz changed %.2f dB, want within 1 dB", change)
	}
}

func TestHighPassBypass(t *testing.T) {
	pcm := sinePCM(16000, 100*time.Millisecond, map[float64]float64{50: 8000})
	filtered := NewHighPass(&memSource{pcm: pcm}, 0, 16000)
	out := make([]byte, len(pcm))
	if err := ReadFull(context.Background(), filtered, out); err != nil {
		t.Fatalf("ReadFull: %v", err)
	}
	if string(out) != string(pcm) {
		t.Fatal("cutoff 0 changed the audio")
	}
}

func TestHighPassKeepsSamplesAcrossReads(t *testing.T) {
	const rate = 16000
	pcm := sinePCM(rate, 100*time.Millisecond, map[float64]float64{50: 8000, 1000: 8000})
	oneShot := NewHighPass(&memSource{pcm: pcm}, 120, rate)
	want := make([]byte, len(pcm))
	if err := ReadFull(context.Background(), oneShot, want); err != nil {
		t.Fatalf("one shot: %v", err)
	}

	chunked := NewHighPass(&memSource{pcm: pcm}, 120, rate)
	got := make([]byte, len(pcm))
	for offset, sizes := 0, []int{2, 14, 200, 6}; offset < len(got); {
		size := min(sizes[0], len(got)-offset)
		if err := ReadFull(context.Background(), chunked, got[offset:offset+size]); err != nil {
			t.Fatalf("chunk at %d: %v", offset, err)
		}
		offset += size
		sizes = append(sizes[1:], sizes[0])
	}
	if string(got) != string(want) {
		t.Fatal("chunked reads changed the filter output")
	}
}
