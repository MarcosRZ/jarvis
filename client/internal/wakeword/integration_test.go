//go:build integration

package wakeword

import (
	"context"
	"log/slog"
	"os"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

// These tests need the Docker services from server/compose.yaml. Piper makes
// the samples. The wake word is "Aleksei", detected by the English "alexa" model.

const (
	wakeModel    = "alexa"
	englishVoice = "en_US-lessac-medium"
	spanishVoice = "es_ES-davefx-medium"
	tailSilence  = 2 * time.Second
)

var integrationTimeouts = wyoming.Timeouts{Connect: 3 * time.Second, Operation: 30 * time.Second}

func serviceAddr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

var (
	wakeAddr = serviceAddr("VOICE_WAKEWORD_ADDR", "127.0.0.1:10400")
	ttsAddr  = serviceAddr("VOICE_TTS_ADDR", "127.0.0.1:10200")
)

// sample is a Source that gives pcm at full speed, then silence at real-time
// pace. done closes when pcm and tailSilence have been read.
type sample struct {
	pcm    []byte
	format audio.Format
	tail   int
	done   chan struct{}
}

func newSample(pcm []byte, format audio.Format) *sample {
	return &sample{pcm: pcm, format: format, tail: format.BytesFor(tailSilence), done: make(chan struct{})}
}

func (s *sample) Read(ctx context.Context, p []byte) (int, error) {
	if len(s.pcm) > 0 {
		n := copy(p, s.pcm)
		s.pcm = s.pcm[n:]
		return n, nil
	}
	select {
	case <-time.After(s.format.Duration(len(p))):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	clear(p)
	if s.tail -= len(p); s.tail <= 0 {
		select {
		case <-s.done:
		default:
			close(s.done)
		}
	}
	return len(p), nil
}

func synthesize(t *testing.T, text, voice string) ([]byte, audio.Format) {
	t.Helper()
	ctx := context.Background()
	c, err := wyoming.Dial(ctx, ttsAddr, integrationTimeouts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	request, _ := wyoming.NewEvent(wyoming.TypeSynthesize, wyoming.Synthesize{Text: text, Voice: &wyoming.Voice{Name: voice}}, nil)
	if err := c.Write(ctx, request); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var format wyoming.AudioFormat
	var pcm []byte
	for {
		ev, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		switch ev.Type {
		case wyoming.TypeAudioStart:
			_ = ev.Decode(&format)
		case wyoming.TypeAudioChunk:
			pcm = append(pcm, ev.Payload...)
		case wyoming.TypeAudioStop:
			return pcm, audio.Format{SampleRate: format.Rate, Channels: format.Channels}
		default:
			t.Fatalf("unexpected %s", ev.Type)
		}
	}
}

// detections streams the sample and returns the detections until the tail silence ends.
func detections(t *testing.T, s *sample) []Detection {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	d := New(Config{Addr: wakeAddr, Model: wakeModel, Cooldown: time.Second, Timeouts: integrationTimeouts, Logger: slog.New(slog.DiscardHandler)}, s.format)
	out := make(chan Detection)
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, s, out) }()

	var got []Detection
	for {
		select {
		case det := <-out:
			t.Logf("detected %s at %v, latency %v", det.Model, det.StreamOffset, det.Latency)
			got = append(got, det)
		case <-s.done:
			cancel()
			<-done
			return got
		case err := <-done:
			t.Fatalf("Run: %v", err)
		}
	}
}

func TestIntegrationWakeWordPositive(t *testing.T) {
	for _, voice := range []string{spanishVoice, englishVoice} {
		t.Run(voice, func(t *testing.T) {
			pcm, format := synthesize(t, "Aleksei.", voice)
			if got := detections(t, newSample(pcm, format)); len(got) != 1 {
				t.Fatalf("got %d detections for \"Aleksei\", want 1", len(got))
			}
		})
	}
}

func TestIntegrationWakeWordNegative(t *testing.T) {
	pcm, format := synthesize(t, "Buenos días. Por favor, enciende la luz de la cocina y dime qué hora es.", spanishVoice)
	if got := detections(t, newSample(pcm, format)); len(got) != 0 {
		t.Fatalf("got %d detections for Spanish speech, want 0", len(got))
	}
}
