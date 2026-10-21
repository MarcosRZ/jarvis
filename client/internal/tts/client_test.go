package tts

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

const testVoice = "es_ES-davefx-medium"

var (
	testTimeouts = wyoming.Timeouts{Connect: time.Second, Operation: time.Second}
	piperFormat  = wyoming.AudioFormat{Rate: 22050, Width: 2, Channels: 1}
)

type event struct {
	typ     string
	data    any
	payload []byte
}

// fakePiper answers one synthesize request with events and reports the request on seen.
func fakePiper(t *testing.T, events ...event) (string, <-chan wyoming.Synthesize) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	seen := make(chan wyoming.Synthesize, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		ev, err := wyoming.ReadEvent(bufio.NewReader(conn))
		if err != nil || ev.Type != wyoming.TypeSynthesize {
			return
		}
		var req wyoming.Synthesize
		_ = ev.Decode(&req)
		seen <- req
		for _, e := range events {
			out, _ := wyoming.NewEvent(e.typ, e.data, e.payload)
			_ = wyoming.WriteEvent(conn, out)
		}
	}()
	return ln.Addr().String(), seen
}

func newClient(addr string) *Client {
	return New(Config{Addr: addr, Voice: testVoice, Timeouts: testTimeouts})
}

func start(f wyoming.AudioFormat) event           { return event{wyoming.TypeAudioStart, f, nil} }
func chunk(f wyoming.AudioFormat, p []byte) event { return event{wyoming.TypeAudioChunk, f, p} }
func stop() event                                 { return event{wyoming.TypeAudioStop, nil, nil} }

func TestSynthesize(t *testing.T) {
	first, second := bytes.Repeat([]byte{1, 2}, 100), bytes.Repeat([]byte{3, 4}, 50)
	addr, seen := fakePiper(t, start(piperFormat), chunk(piperFormat, first), chunk(piperFormat, second), stop())

	speech, err := newClient(addr).Synthesize(context.Background(), "  Son las diez.  ")
	if err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if speech.Format != (audio.Format{SampleRate: 22050, Channels: 1}) {
		t.Errorf("format %+v, want the rate from audio-start", speech.Format)
	}
	if !bytes.Equal(speech.PCM, append(first, second...)) {
		t.Errorf("got %d bytes, want both chunks in order", len(speech.PCM))
	}
	req := <-seen
	if req.Text != "Son las diez." || req.Voice == nil || req.Voice.Name != testVoice || req.Voice.Speaker != "" {
		t.Errorf("request %+v, voice %+v", req, req.Voice)
	}
}

func TestSynthesizeSendsSpeaker(t *testing.T) {
	addr, seen := fakePiper(t, start(piperFormat), chunk(piperFormat, []byte{1, 2}), stop())
	c := New(Config{Addr: addr, Voice: "es_ES-sharvard-medium", Speaker: "F", Timeouts: testTimeouts})
	if _, err := c.Synthesize(context.Background(), "Hola"); err != nil {
		t.Fatalf("Synthesize: %v", err)
	}
	if req := <-seen; req.Voice == nil || req.Voice.Name != "es_ES-sharvard-medium" || req.Voice.Speaker != "F" {
		t.Errorf("voice %+v, want es_ES-sharvard-medium speaker F", req.Voice)
	}
}

func TestSynthesizeRejectsEmptyTextWithoutCallingPiper(t *testing.T) {
	for _, text := range []string{"", "  \n "} {
		if _, err := newClient("127.0.0.1:1").Synthesize(context.Background(), text); !errors.Is(err, ErrEmptyText) {
			t.Errorf("text %q: error %v, want ErrEmptyText", text, err)
		}
	}
}

func TestSynthesizeRejectsLongText(t *testing.T) {
	long := string(bytes.Repeat([]byte("a"), maxTextBytes+1))
	if _, err := newClient("127.0.0.1:1").Synthesize(context.Background(), long); !errors.Is(err, ErrTextTooLong) {
		t.Fatalf("error %v, want ErrTextTooLong", err)
	}
}

func TestSynthesizeServiceError(t *testing.T) {
	addr, _ := fakePiper(t, event{wyoming.TypeError, wyoming.Error{Text: "xx_XX-nobody", Code: "VoiceNotFoundError"}, nil})
	_, err := newClient(addr).Synthesize(context.Background(), "Hola")
	var se *wyoming.ServiceError
	if !errors.As(err, &se) || se.Code != "VoiceNotFoundError" {
		t.Fatalf("error %v, want the service error", err)
	}
}

func TestSynthesizeNoAudio(t *testing.T) {
	addr, _ := fakePiper(t, stop())
	if _, err := newClient(addr).Synthesize(context.Background(), "Hola"); !errors.Is(err, ErrNoAudio) {
		t.Fatalf("error %v, want ErrNoAudio", err)
	}
}

func TestSynthesizeRejectsMalformedAudio(t *testing.T) {
	frames := []byte{1, 2, 3, 4}
	tests := map[string][]event{
		"chunk before start":   {chunk(piperFormat, frames), stop()},
		"8-bit samples":        {start(wyoming.AudioFormat{Rate: 22050, Width: 1, Channels: 1}), stop()},
		"no channels":          {start(wyoming.AudioFormat{Rate: 22050, Width: 2, Channels: 0}), stop()},
		"no rate":              {start(wyoming.AudioFormat{Rate: 0, Width: 2, Channels: 1}), stop()},
		"chunk format changes": {start(piperFormat), chunk(wyoming.AudioFormat{Rate: 16000, Width: 2, Channels: 1}, frames), stop()},
		"partial frame":        {start(piperFormat), chunk(piperFormat, []byte{1, 2, 3}), stop()},
		"second start":         {start(piperFormat), chunk(piperFormat, frames), start(piperFormat), stop()},
		"unexpected event":     {start(piperFormat), event{wyoming.TypeTranscript, wyoming.Transcript{Text: "x"}, nil}},
	}
	for name, events := range tests {
		t.Run(name, func(t *testing.T) {
			addr, _ := fakePiper(t, events...)
			if _, err := newClient(addr).Synthesize(context.Background(), "Hola"); !errors.Is(err, ErrMalformedAudio) && !errors.Is(err, wyoming.ErrUnexpectedEvent) {
				t.Fatalf("error %v, want ErrMalformedAudio or ErrUnexpectedEvent", err)
			}
		})
	}
}

func TestSynthesizeRejectsTooMuchAudio(t *testing.T) {
	limit := (audio.Format{SampleRate: 22050, Channels: 1}).BytesFor(maxSpeech)
	big := make([]byte, 1<<20)
	events := []event{start(piperFormat)}
	for sent := 0; sent <= limit; sent += len(big) {
		events = append(events, chunk(piperFormat, big))
	}
	addr, _ := fakePiper(t, append(events, stop())...)
	c := New(Config{Addr: addr, Voice: testVoice, Timeouts: wyoming.Timeouts{Connect: time.Second, Operation: 5 * time.Second}})
	if _, err := c.Synthesize(context.Background(), "Hola"); !errors.Is(err, ErrSpeechTooLong) {
		t.Fatalf("error %v, want ErrSpeechTooLong", err)
	}
}

func TestSynthesizeConnectionClosedBeforeStop(t *testing.T) {
	addr, _ := fakePiper(t, start(piperFormat), chunk(piperFormat, []byte{1, 2}))
	if _, err := newClient(addr).Synthesize(context.Background(), "Hola"); !errors.Is(err, wyoming.ErrConnectionClosed) {
		t.Fatalf("error %v, want ErrConnectionClosed", err)
	}
}
