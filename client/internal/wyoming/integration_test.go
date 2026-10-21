//go:build integration

package wyoming

import (
	"context"
	"os"
	"slices"
	"strings"
	"testing"
	"time"
)

// These tests need the Docker services from server/compose.yaml.

const (
	spanishVoice = "es_ES-davefx-medium"
	wakeModel    = "alexa"
	spanish      = "es"
)

var integrationTimeouts = Timeouts{Connect: 3 * time.Second, Operation: 60 * time.Second}

func serviceAddr(env, fallback string) string {
	if v := os.Getenv(env); v != "" {
		return v
	}
	return fallback
}

var (
	wakeAddr = serviceAddr("VOICE_WAKEWORD_ADDR", "127.0.0.1:10400")
	sttAddr  = serviceAddr("VOICE_STT_ADDR", "127.0.0.1:10300")
	ttsAddr  = serviceAddr("VOICE_TTS_ADDR", "127.0.0.1:10200")
)

func modelNames(programs []Program) []string {
	var names []string
	for _, p := range programs {
		for _, m := range append(p.Models, p.Voices...) {
			names = append(names, m.Name)
		}
	}
	return names
}

func TestIntegrationDescribe(t *testing.T) {
	ctx := context.Background()
	tests := []struct {
		name     string
		addr     string
		programs func(Info) []Program
		want     string
	}{
		{"openwakeword", wakeAddr, func(i Info) []Program { return i.Wake }, wakeModel},
		{"whisper", sttAddr, func(i Info) []Program { return i.ASR }, ""},
		{"piper", ttsAddr, func(i Info) []Program { return i.TTS }, spanishVoice},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			info, err := Describe(ctx, tt.addr, integrationTimeouts)
			if err != nil {
				t.Fatalf("Describe: %v", err)
			}
			programs := tt.programs(info)
			if len(programs) == 0 {
				t.Fatalf("no programs in %+v", info)
			}
			if tt.want != "" && !slices.Contains(modelNames(programs), tt.want) {
				t.Errorf("%s not in %v", tt.want, modelNames(programs))
			}
		})
	}
}

// TestIntegrationSpeechRoundTrip makes Piper speak a Spanish sentence and
// sends the audio to Whisper, so binary payloads cross both services.
func TestIntegrationSpeechRoundTrip(t *testing.T) {
	ctx := context.Background()
	// Piper output changes on each run and Whisper base is weak on short
	// phrases, so the sentence is long and the check uses one clear word.
	format, pcm := synthesize(t, ctx, "Buenos días. Por favor, enciende la luz de la cocina.")
	if len(pcm) == 0 {
		t.Fatal("Piper returned no audio")
	}

	text := transcribe(t, ctx, format, pcm)
	if !strings.Contains(strings.ToLower(text), "cocina") {
		t.Fatalf("transcript %q does not contain \"cocina\"", text)
	}
}

func synthesize(t *testing.T, ctx context.Context, text string) (AudioFormat, []byte) {
	t.Helper()
	c, err := Dial(ctx, ttsAddr, integrationTimeouts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	request, _ := NewEvent(TypeSynthesize, Synthesize{Text: text, Voice: &Voice{Name: spanishVoice}}, nil)
	if err := c.Write(ctx, request); err != nil {
		t.Fatalf("Write: %v", err)
	}
	var format AudioFormat
	var pcm []byte
	for {
		ev, err := c.Read(ctx)
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
		if err := CheckType(ev, TypeAudioStart, TypeAudioChunk, TypeAudioStop); err != nil {
			t.Fatal(err)
		}
		switch ev.Type {
		case TypeAudioStart:
			if err := ev.Decode(&format); err != nil {
				t.Fatal(err)
			}
		case TypeAudioChunk:
			pcm = append(pcm, ev.Payload...)
		case TypeAudioStop:
			return format, pcm
		}
	}
}

func transcribe(t *testing.T, ctx context.Context, format AudioFormat, pcm []byte) string {
	t.Helper()
	c, err := Dial(ctx, sttAddr, integrationTimeouts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer c.Close()

	const chunkBytes = 4096
	events := []Event{}
	add := func(typ string, data any, payload []byte) {
		ev, err := NewEvent(typ, data, payload)
		if err != nil {
			t.Fatal(err)
		}
		events = append(events, ev)
	}
	add(TypeTranscribe, Transcribe{Language: spanish}, nil)
	add(TypeAudioStart, format, nil)
	for start := 0; start < len(pcm); start += chunkBytes {
		add(TypeAudioChunk, format, pcm[start:min(start+chunkBytes, len(pcm))])
	}
	add(TypeAudioStop, nil, nil)
	for _, ev := range events {
		if err := c.Write(ctx, ev); err != nil {
			t.Fatalf("Write: %v", err)
		}
	}

	ev, err := c.Read(ctx)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if err := CheckType(ev, TypeTranscript); err != nil {
		t.Fatal(err)
	}
	var tr Transcript
	if err := ev.Decode(&tr); err != nil {
		t.Fatal(err)
	}
	t.Logf("Piper %d Hz, %d bytes -> Whisper %q", format.Rate, len(pcm), tr.Text)
	return tr.Text
}
