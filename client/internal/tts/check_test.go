package tts

import (
	"bufio"
	"context"
	"errors"
	"net"
	"strings"
	"testing"

	"jarvis/client/internal/wyoming"
)

// fakeDescribe answers describe requests with voices.
func fakeDescribe(t *testing.T, voices ...wyoming.Model) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if ev, err := wyoming.ReadEvent(bufio.NewReader(conn)); err == nil && ev.Type == wyoming.TypeDescribe {
				info, _ := wyoming.NewEvent(wyoming.TypeInfo, wyoming.Info{TTS: []wyoming.Program{{Name: "piper", Voices: voices}}}, nil)
				_ = wyoming.WriteEvent(conn, info)
			}
			conn.Close()
		}
	}()
	return ln.Addr().String()
}

var sharvard = wyoming.Model{Name: "es_ES-sharvard-medium", Speakers: []wyoming.Speaker{{Name: "M"}, {Name: "F"}}}

func check(addr, voice, speaker string) error {
	return New(Config{Addr: addr, Voice: voice, Speaker: speaker, Timeouts: testTimeouts}).Check(context.Background())
}

func TestCheckAcceptsKnownSpeaker(t *testing.T) {
	addr := fakeDescribe(t, wyoming.Model{Name: testVoice}, sharvard)
	if err := check(addr, "es_ES-sharvard-medium", "F"); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckWithoutSpeakerNeedsNoService(t *testing.T) {
	if err := check("127.0.0.1:1", testVoice, ""); err != nil {
		t.Fatalf("Check: %v", err)
	}
}

func TestCheckRejectsUnknownSpeaker(t *testing.T) {
	addr := fakeDescribe(t, sharvard)
	err := check(addr, "es_ES-sharvard-medium", "X")
	if !errors.Is(err, ErrUnknownSpeaker) {
		t.Fatalf("error %v, want ErrUnknownSpeaker", err)
	}
	if !strings.Contains(err.Error(), "M, F") {
		t.Errorf("error %q does not list the speakers", err)
	}
}

func TestCheckRejectsSpeakerForSingleSpeakerVoice(t *testing.T) {
	addr := fakeDescribe(t, wyoming.Model{Name: testVoice})
	if err := check(addr, testVoice, "F"); !errors.Is(err, ErrUnknownSpeaker) {
		t.Fatalf("error %v, want ErrUnknownSpeaker", err)
	}
}

func TestCheckRejectsUnknownVoice(t *testing.T) {
	addr := fakeDescribe(t, sharvard)
	if err := check(addr, "xx_XX-nobody-medium", "F"); !errors.Is(err, ErrUnknownVoice) {
		t.Fatalf("error %v, want ErrUnknownVoice", err)
	}
}
