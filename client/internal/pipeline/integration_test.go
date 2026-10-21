//go:build integration

package pipeline

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/commands"
	"jarvis/client/internal/homeassistant"
	"jarvis/client/internal/stt"
	"jarvis/client/internal/tts"
	"jarvis/client/internal/wakeword"
	"jarvis/client/internal/wyoming"
)

// This test needs the Docker services from server/compose.yaml. A paced source
// plays Piper speech like a microphone: "Aleksei", a pause, then a command.
// All real adapters run; only the speaker output is a fake.

const testSpeechVoice = "es_ES-davefx-medium"

var integrationTimeouts = wyoming.Timeouts{Connect: 3 * time.Second, Operation: 30 * time.Second}

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

// pacedSource gives pcm at real-time speed, then silence.
type pacedSource struct {
	mu     sync.Mutex
	pcm    []byte
	format audio.Format
}

func (s *pacedSource) Read(ctx context.Context, p []byte) (int, error) {
	n := min(len(p), s.format.BytesFor(20*time.Millisecond))
	select {
	case <-time.After(s.format.Duration(n)):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	k := copy(p[:n], s.pcm)
	s.pcm = s.pcm[k:]
	clear(p[k:n])
	return n, nil
}

type discardPlayer struct{}

func (discardPlayer) Write(context.Context, []byte) error { return nil }
func (discardPlayer) Drain(context.Context) error         { return nil }
func (discardPlayer) Close()                              {}

func speech(t *testing.T, client *tts.Client, text string) ([]byte, audio.Format) {
	t.Helper()
	s, err := client.Synthesize(context.Background(), text)
	if err != nil {
		t.Fatalf("Synthesize(%q): %v", text, err)
	}
	return s.PCM, s.Format
}

// conversation is said in order, each command after "Aleksei"; want is the
// start of each reply.
var conversation = []struct{ command, want string }{
	{"¿Qué hora es?", "Son las 10 y cuarto."},
	{"¿Qué puedes hacer?", "Puedo decirte la hora"},
	{"Repite lo que he dicho.", "Has dicho: "},
}

func TestIntegrationConversation(t *testing.T) {
	handler, err := commands.New(func() time.Time { return time.Date(2026, 10, 20, 22, 15, 0, 0, time.Local) }, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	var spoken []string
	for _, step := range conversation {
		spoken = append(spoken, step.command)
	}
	heard, said := converse(t, handler, spoken)
	for i, step := range conversation {
		if !strings.HasPrefix(said[i], step.want) {
			t.Errorf("reply %d %q, want it to start with %q", i+1, said[i], step.want)
		}
	}
	if want := "Has dicho: " + heard[1]; said[2] != want {
		t.Errorf("repeat reply %q, want %q", said[2], want)
	}
}

// TestIntegrationDeviceCommand needs the development Home Assistant (make ha-up).
func TestIntegrationDeviceCommand(t *testing.T) {
	url, token := os.Getenv("VOICE_HA_URL"), os.Getenv("VOICE_HA_TOKEN")
	if url == "" || token == "" {
		t.Skip("VOICE_HA_URL and VOICE_HA_TOKEN are not set")
	}
	ha := homeassistant.New(homeassistant.Config{URL: url, Token: token, Timeout: 5 * time.Second})
	const kitchen = "light.kitchen_lights"
	if err := ha.SetPower(context.Background(), kitchen, false); err != nil {
		t.Fatal(err)
	}
	handler, err := commands.New(time.Now, nil, ha)
	if err != nil {
		t.Fatal(err)
	}
	handler.UseSource(ha, time.Minute, func(err error) { t.Logf("devices: %v", err) })

	_, said := converse(t, handler, []string{"Enciende la luz de la cocina."})
	if said[0] != commands.ReplyDone {
		t.Errorf("reply %q", said[0])
	}
	devices, err := ha.Devices(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got := lightIsOn(t, url, token, kitchen); !got {
		t.Errorf("%s is not on after the voice command; devices %v", kitchen, devices)
	}
}

func lightIsOn(t *testing.T, url, token, id string) bool {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, url+"/api/states/"+id, nil)
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var s struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		t.Fatal(err)
	}
	return s.State == "on"
}

// converse says each command after "Aleksei" through the real services and
// returns what Whisper heard and what the assistant replied.
func converse(t *testing.T, handler *commands.Handler, spoken []string) (heard, said []string) {
	t.Helper()
	voice := tts.New(tts.Config{Addr: ttsAddr, Voice: testSpeechVoice, Timeouts: integrationTimeouts})
	wake, format := speech(t, voice, "Aleksei.")
	silence := func(d time.Duration) []byte { return make([]byte, format.BytesFor(d)) }
	input := silence(time.Second)
	for _, text := range spoken {
		command, _ := speech(t, voice, text)
		for _, part := range [][]byte{wake, silence(time.Second), command, silence(4 * time.Second)} {
			input = append(input, part...)
		}
	}
	src := &pacedSource{pcm: input, format: format}

	logger := slog.New(slog.DiscardHandler)
	detector := wakeword.New(wakeword.Config{Addr: wakeAddr, Model: "alexa", Cooldown: 2 * time.Second, Timeouts: integrationTimeouts, Logger: logger}, format)
	transcriber := stt.New(stt.Config{Addr: sttAddr, Language: "es", Timeouts: integrationTimeouts})
	reply := tts.NewSpeaker(
		tts.New(tts.Config{Addr: ttsAddr, Voice: "es_ES-sharvard-medium", Speaker: "F", Timeouts: integrationTimeouts}),
		func(audio.Format) (tts.Player, error) { return discardPlayer{}, nil },
	)
	limits := audio.RecordLimits{Max: 8 * time.Second, StartTimeout: 3 * time.Second, EndSilence: time.Second, SpeechLevel: -40}

	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	p := New(Steps{
		WaitForWakeWord: func(ctx context.Context) (wakeword.Detection, error) { return detector.Wait(ctx, src) },
		Acknowledge: func(ctx context.Context) error {
			_, err := reply.Play(ctx, audio.BeepFormat, audio.DefaultBeep.PCM())
			return err
		},
		Record: func(ctx context.Context) (audio.Recording, error) {
			return audio.Record(ctx, src, format, limits)
		},
		Transcribe: func(ctx context.Context, f audio.Format, pcm []byte) (stt.Result, error) {
			res, err := transcriber.Transcribe(ctx, f, pcm)
			heard = append(heard, res.Text)
			return res, err
		},
		Handle: handler.Handle,
		Say: func(ctx context.Context, text string) (tts.Timing, error) {
			said = append(said, text)
			if len(said) == len(spoken) {
				defer cancel()
			}
			return reply.Say(ctx, text)
		},
		DiscardInput: func() {},
	}, logger)

	start := time.Now()
	_ = p.Run(ctx)
	t.Logf("%v for %d requests", time.Since(start).Round(10*time.Millisecond), len(spoken))
	for i := range said {
		t.Logf("heard %q, replied %q", heard[i], said[i])
	}
	if len(said) != len(spoken) {
		t.Fatalf("%d replies, want %d", len(said), len(spoken))
	}
	return heard, said
}
