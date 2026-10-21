//go:build integration

package tts

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

// These tests need the Docker services from server/compose.yaml.

var integrationTimeouts = wyoming.Timeouts{Connect: 3 * time.Second, Operation: 30 * time.Second}

func ttsAddr() string {
	if v := os.Getenv("VOICE_TTS_ADDR"); v != "" {
		return v
	}
	return "127.0.0.1:10200"
}

func TestIntegrationSynthesizeSequential(t *testing.T) {
	c := New(Config{Addr: ttsAddr(), Voice: testVoice, Timeouts: integrationTimeouts})
	for _, text := range []string{"Son las diez y cuarto.", "No te he entendido.", "Hola."} {
		started := time.Now()
		speech, err := c.Synthesize(context.Background(), text)
		if err != nil {
			t.Fatalf("Synthesize(%q): %v", text, err)
		}
		length := speech.Format.Duration(len(speech.PCM))
		t.Logf("%-26q %v of audio at %d Hz in %v", text, length.Round(10*time.Millisecond), speech.Format.SampleRate, time.Since(started).Round(time.Millisecond))
		if speech.Format != (audio.Format{SampleRate: 22050, Channels: 1}) {
			t.Errorf("format %+v, want 22050 Hz mono for %s", speech.Format, testVoice)
		}
		if length < 300*time.Millisecond || length > 10*time.Second {
			t.Errorf("%v of audio for %q", length, text)
		}
	}
}

func TestIntegrationSpeaker(t *testing.T) {
	const voice = "es_ES-sharvard-medium"
	female := New(Config{Addr: ttsAddr(), Voice: voice, Speaker: "F", Timeouts: integrationTimeouts})
	if err := female.Check(context.Background()); err != nil {
		t.Fatalf("Check speaker F: %v", err)
	}
	if _, err := female.Synthesize(context.Background(), "Hola, soy Aleksei."); err != nil {
		t.Fatalf("Synthesize with speaker F: %v", err)
	}
	unknown := New(Config{Addr: ttsAddr(), Voice: voice, Speaker: "X", Timeouts: integrationTimeouts})
	if err := unknown.Check(context.Background()); !errors.Is(err, ErrUnknownSpeaker) {
		t.Fatalf("Check speaker X: error %v, want ErrUnknownSpeaker", err)
	}
}

func TestIntegrationUnknownVoice(t *testing.T) {
	c := New(Config{Addr: ttsAddr(), Voice: "xx_XX-nobody-medium", Timeouts: integrationTimeouts})
	_, err := c.Synthesize(context.Background(), "Hola")
	var se *wyoming.ServiceError
	if !errors.As(err, &se) || se.Code != "VoiceNotFoundError" {
		t.Fatalf("error %v, want a VoiceNotFoundError service error", err)
	}
}
