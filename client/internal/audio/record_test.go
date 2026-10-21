package audio

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

// Levels for the test signal: speech is a square wave at -20 dBFS, quiet is
// noise at -60 dBFS, and the speech level is -40 dBFS.
const (
	loudSample  = 3277
	quietSample = 33
)

var testLimits = RecordLimits{
	Max:          4 * time.Second,
	StartTimeout: time.Second,
	EndSilence:   500 * time.Millisecond,
	SpeechLevel:  -40,
}

// testAudio builds SpeechFormat PCM from parts of loud and quiet audio.
type part struct {
	loud bool
	d    time.Duration
}

func testAudio(parts ...part) []byte {
	var out []byte
	for _, p := range parts {
		sample := int16(quietSample)
		if p.loud {
			sample = loudSample
		}
		n := SpeechFormat.BytesFor(p.d) / BytesPerSample
		for i := range n {
			s := sample
			if i%2 == 1 {
				s = -s
			}
			out = append(out, byte(s), byte(uint16(s)>>8))
		}
	}
	return out
}

// memSource gives pcm in reads of at most 10 ms, then blocks until ctx ends,
// like a live microphone.
type memSource struct {
	pcm []byte
	err error
}

func (m *memSource) Read(ctx context.Context, p []byte) (int, error) {
	if len(m.pcm) == 0 {
		if m.err != nil {
			return 0, m.err
		}
		<-ctx.Done()
		return 0, ctx.Err()
	}
	n := copy(p[:min(len(p), SpeechFormat.BytesFor(10*time.Millisecond))], m.pcm)
	m.pcm = m.pcm[n:]
	return n, nil
}

func record(t *testing.T, pcm []byte) Recording {
	t.Helper()
	rec, err := Record(context.Background(), &memSource{pcm: pcm}, SpeechFormat, testLimits)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	return rec
}

func TestRecordEndsAfterSilence(t *testing.T) {
	input := testAudio(part{false, 300 * time.Millisecond}, part{true, time.Second}, part{false, 2 * time.Second})
	rec := record(t, input)

	if rec.End != EndSilence {
		t.Fatalf("end %q, want %q", rec.End, EndSilence)
	}
	if got, want := SpeechFormat.Duration(len(rec.PCM)), 1800*time.Millisecond; got != want {
		t.Errorf("recording %v, want %v (lead, speech and end silence)", got, want)
	}
	if rec.SpeechStart != 300*time.Millisecond {
		t.Errorf("speech start %v, want 300ms", rec.SpeechStart)
	}
	if !bytes.Equal(rec.PCM, input[:len(rec.PCM)]) {
		t.Error("recording is not the start of the input")
	}
}

func TestRecordKeepsShortPauses(t *testing.T) {
	rec := record(t, testAudio(
		part{true, 500 * time.Millisecond}, part{false, 400 * time.Millisecond},
		part{true, 500 * time.Millisecond}, part{false, time.Second},
	))
	if rec.End != EndSilence {
		t.Fatalf("end %q, want %q", rec.End, EndSilence)
	}
	if got, want := SpeechFormat.Duration(len(rec.PCM)), 1900*time.Millisecond; got != want {
		t.Errorf("recording %v, want %v: a pause shorter than EndSilence must not end it", got, want)
	}
}

func TestRecordStopsAtMax(t *testing.T) {
	rec := record(t, testAudio(part{true, 10 * time.Second}))
	if rec.End != EndMax {
		t.Fatalf("end %q, want %q", rec.End, EndMax)
	}
	if got := SpeechFormat.Duration(len(rec.PCM)); got != testLimits.Max {
		t.Errorf("recording %v, want %v", got, testLimits.Max)
	}
}

func TestRecordWithoutSpeech(t *testing.T) {
	rec := record(t, testAudio(part{false, 5 * time.Second}))
	if rec.End != EndNoSpeech {
		t.Fatalf("end %q, want %q", rec.End, EndNoSpeech)
	}
	if len(rec.PCM) != 0 {
		t.Errorf("kept %d bytes of audio with no speech", len(rec.PCM))
	}
}

func TestRecordSpeechJustBeforeStartTimeout(t *testing.T) {
	rec := record(t, testAudio(part{false, 900 * time.Millisecond}, part{true, 300 * time.Millisecond}, part{false, time.Second}))
	if rec.End != EndSilence {
		t.Fatalf("end %q, want %q", rec.End, EndSilence)
	}
}

func TestRecordReturnsSourceError(t *testing.T) {
	gone := errors.New("device gone")
	_, err := Record(context.Background(), &memSource{pcm: testAudio(part{true, 200 * time.Millisecond}), err: gone}, SpeechFormat, testLimits)
	if !errors.Is(err, gone) {
		t.Fatalf("error %v, want %v", err, gone)
	}
}

func TestRecordStopsOnCancel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Record(ctx, &memSource{pcm: testAudio(part{true, 200 * time.Millisecond})}, SpeechFormat, testLimits)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("error %v, want deadline exceeded", err)
	}
}
