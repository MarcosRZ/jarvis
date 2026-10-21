package tts

import (
	"bytes"
	"context"
	"errors"
	"slices"
	"sync"
	"testing"
	"time"

	"jarvis/client/internal/audio"
)

// fakeOutput records the playback lifecycle as a list of steps.
type fakeOutput struct {
	mu       sync.Mutex
	steps    []string
	formats  []audio.Format
	played   []byte
	openErr  error
	writeErr error
	hang     bool
}

func (o *fakeOutput) open(format audio.Format) (Player, error) {
	o.mu.Lock()
	defer o.mu.Unlock()
	o.steps = append(o.steps, "open")
	o.formats = append(o.formats, format)
	if o.openErr != nil {
		return nil, o.openErr
	}
	return &fakePlayer{o}, nil
}

func (o *fakeOutput) record(step string) {
	o.mu.Lock()
	o.steps = append(o.steps, step)
	o.mu.Unlock()
}

func (o *fakeOutput) lifecycle() []string {
	o.mu.Lock()
	defer o.mu.Unlock()
	return slices.Clone(o.steps)
}

type fakePlayer struct{ o *fakeOutput }

func (p *fakePlayer) Write(_ context.Context, pcm []byte) error {
	p.o.record("write")
	if p.o.writeErr != nil {
		return p.o.writeErr
	}
	p.o.mu.Lock()
	p.o.played = append(p.o.played, pcm...)
	p.o.mu.Unlock()
	return nil
}

func (p *fakePlayer) Drain(ctx context.Context) error {
	p.o.record("drain")
	if p.o.hang {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

func (p *fakePlayer) Close() { p.o.record("close") }

func speakerFor(t *testing.T, out *fakeOutput, pcm []byte) *Speaker {
	t.Helper()
	addr, _ := fakePiper(t, start(piperFormat), chunk(piperFormat, pcm), stop())
	s := NewSpeaker(newClient(addr), out.open)
	s.playMargin = 50 * time.Millisecond
	return s
}

func TestSayPlaysSpeechInItsFormat(t *testing.T) {
	pcm := bytes.Repeat([]byte{5, 6}, 2205)
	out := &fakeOutput{}

	timing, err := speakerFor(t, out, pcm).Say(context.Background(), "Hola")
	if err != nil {
		t.Fatalf("Say: %v", err)
	}
	if got := out.lifecycle(); !slices.Equal(got, []string{"open", "write", "drain", "close"}) {
		t.Errorf("lifecycle %v", got)
	}
	if out.formats[0] != (audio.Format{SampleRate: 22050, Channels: 1}) {
		t.Errorf("opened with %+v, want the Piper format", out.formats[0])
	}
	if !bytes.Equal(out.played, pcm) {
		t.Errorf("played %d bytes, want %d", len(out.played), len(pcm))
	}
	if timing.Audio != 100*time.Millisecond || timing.Synthesis <= 0 {
		t.Errorf("timing %+v", timing)
	}
}

func TestSayClosesAfterWriteError(t *testing.T) {
	broken := errors.New("device gone")
	out := &fakeOutput{writeErr: broken}
	_, err := speakerFor(t, out, []byte{1, 2}).Say(context.Background(), "Hola")
	if !errors.Is(err, broken) {
		t.Fatalf("error %v, want %v", err, broken)
	}
	if got := out.lifecycle(); !slices.Equal(got, []string{"open", "write", "close"}) {
		t.Errorf("lifecycle %v", got)
	}
}

func TestSayReportsOpenError(t *testing.T) {
	noDevice := errors.New("no device")
	out := &fakeOutput{openErr: noDevice}
	if _, err := speakerFor(t, out, []byte{1, 2}).Say(context.Background(), "Hola"); !errors.Is(err, noDevice) {
		t.Fatalf("error %v, want %v", err, noDevice)
	}
}

func TestSayDoesNotHangOnStuckPlayback(t *testing.T) {
	out := &fakeOutput{hang: true}
	start := time.Now()
	_, err := speakerFor(t, out, bytes.Repeat([]byte{1, 2}, 2205)).Say(context.Background(), "Hola")
	if !errors.Is(err, ErrPlaybackStuck) {
		t.Fatalf("error %v, want ErrPlaybackStuck", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("Say took %v for 100 ms of audio and a 50 ms margin", elapsed)
	}
	if got := out.lifecycle(); got[len(got)-1] != "close" {
		t.Errorf("lifecycle %v does not end with close", got)
	}
}

func TestSayStopsOnCancel(t *testing.T) {
	out := &fakeOutput{hang: true}
	s := speakerFor(t, out, bytes.Repeat([]byte{1, 2}, 22050*10))
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(100*time.Millisecond, cancel)

	if _, err := s.Say(ctx, "Hola"); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
	if got := out.lifecycle(); got[len(got)-1] != "close" {
		t.Errorf("lifecycle %v does not end with close", got)
	}
}

func TestSayDoesNotOpenOutputWhenSynthesisFails(t *testing.T) {
	out := &fakeOutput{}
	s := NewSpeaker(newClient("127.0.0.1:1"), out.open)
	if _, err := s.Say(context.Background(), "Hola"); err == nil {
		t.Fatal("no error from an unreachable Piper")
	}
	if got := out.lifecycle(); len(got) != 0 {
		t.Errorf("lifecycle %v, want no playback", got)
	}
}

func TestPlayPlaysPCMInItsFormat(t *testing.T) {
	out := &fakeOutput{}
	s := NewSpeaker(newClient("127.0.0.1:1"), out.open)
	pcm := audio.DefaultBeep.PCM()
	if _, err := s.Play(context.Background(), audio.BeepFormat, pcm); err != nil {
		t.Fatalf("Play: %v", err)
	}
	if got := out.lifecycle(); !slices.Equal(got, []string{"open", "write", "drain", "close"}) {
		t.Errorf("lifecycle %v", got)
	}
	if out.formats[0] != audio.BeepFormat || !bytes.Equal(out.played, pcm) {
		t.Errorf("played %d bytes as %+v", len(out.played), out.formats[0])
	}
}
