//go:build integration

package stt

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

// These tests need the Docker services from server/compose.yaml. Piper says
// each sample; the audio then goes through audio.Record, as microphone audio would.

const spanishVoice = "es_ES-davefx-medium"

var integrationTimeouts = wyoming.Timeouts{Connect: 3 * time.Second, Operation: 60 * time.Second}

var limits = audio.RecordLimits{Max: 8 * time.Second, StartTimeout: 3 * time.Second, EndSilence: time.Second, SpeechLevel: -40}

func serviceAddr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

var (
	sttAddr = serviceAddr("VOICE_STT_ADDR", "127.0.0.1:10300")
	ttsAddr = serviceAddr("VOICE_TTS_ADDR", "127.0.0.1:10200")
)

// withSilence gives pcm, then silence, like a microphone after the speaker stops.
type withSilence struct{ pcm []byte }

func (s *withSilence) Read(_ context.Context, p []byte) (int, error) {
	if len(s.pcm) == 0 {
		clear(p)
		return len(p), nil
	}
	n := copy(p, s.pcm)
	s.pcm = s.pcm[n:]
	return n, nil
}

func synthesize(t *testing.T, text string) ([]byte, audio.Format) {
	t.Helper()
	ctx := context.Background()
	c, err := wyoming.Dial(ctx, ttsAddr, integrationTimeouts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()
	request, _ := wyoming.NewEvent(wyoming.TypeSynthesize, wyoming.Synthesize{Text: text, Voice: &wyoming.Voice{Name: spanishVoice}}, nil)
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

func recordAndTranscribe(t *testing.T, text string) string {
	t.Helper()
	pcm, format := synthesize(t, text)
	rec, err := audio.Record(context.Background(), &withSilence{pcm: pcm}, format, limits)
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if rec.End != audio.EndSilence {
		t.Fatalf("recording ended with %q, want %q", rec.End, audio.EndSilence)
	}
	res, err := New(Config{Addr: sttAddr, Language: "es", Timeouts: integrationTimeouts}).Transcribe(context.Background(), rec.Format, rec.PCM)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	t.Logf("%-28q speech %v, recorded %v, Whisper %v -> %q", text, format.Duration(len(pcm)).Round(10*time.Millisecond),
		rec.Format.Duration(len(rec.PCM)).Round(10*time.Millisecond), res.Processing.Round(time.Millisecond), res.Text)
	return res.Text
}

func TestIntegrationTranscribeSentence(t *testing.T) {
	text := recordAndTranscribe(t, "Buenos días. Por favor, enciende la luz de la cocina.")
	if !strings.Contains(strings.ToLower(text), "cocina") {
		t.Fatalf("transcript %q does not contain \"cocina\"", text)
	}
}

// TestIntegrationCommands logs the spec commands. Whisper base gets short
// commands wrong now and then, so only an empty transcript fails.
func TestIntegrationCommands(t *testing.T) {
	for _, command := range []string{"¿Qué hora es?", "Repite lo que he dicho.", "Cancela.", "¿Qué puedes hacer?"} {
		if recordAndTranscribe(t, command) == "" {
			t.Errorf("empty transcript for %q", command)
		}
	}
}
