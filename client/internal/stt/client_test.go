package stt

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

var testTimeouts = wyoming.Timeouts{Connect: time.Second, Operation: time.Second}

type request struct {
	language string
	format   wyoming.AudioFormat
	audio    []byte
	stopped  bool
}

// fakeWhisper reads one request, reports it on seen, and answers with reply.
func fakeWhisper(t *testing.T, reply func(conn net.Conn)) (string, <-chan request) {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	seen := make(chan request, 1)
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		r := bufio.NewReader(conn)
		var req request
		for !req.stopped {
			ev, err := wyoming.ReadEvent(r)
			if err != nil {
				return
			}
			switch ev.Type {
			case wyoming.TypeTranscribe:
				var tr wyoming.Transcribe
				_ = ev.Decode(&tr)
				req.language = tr.Language
			case wyoming.TypeAudioStart:
				_ = ev.Decode(&req.format)
			case wyoming.TypeAudioChunk:
				req.audio = append(req.audio, ev.Payload...)
			case wyoming.TypeAudioStop:
				req.stopped = true
			}
		}
		seen <- req
		reply(conn)
	}()
	return ln.Addr().String(), seen
}

func send(conn net.Conn, typ string, data any) {
	ev, _ := wyoming.NewEvent(typ, data, nil)
	_ = wyoming.WriteEvent(conn, ev)
}

func newClient(addr string) *Client {
	return New(Config{Addr: addr, Language: "es", Timeouts: testTimeouts})
}

func TestTranscribe(t *testing.T) {
	addr, seen := fakeWhisper(t, func(conn net.Conn) {
		send(conn, wyoming.TypeTranscript, wyoming.Transcript{Text: " ¿Qué hora es? "})
	})
	pcm := bytes.Repeat([]byte{1, 2}, audio.SpeechFormat.BytesFor(time.Second)/2)

	res, err := newClient(addr).Transcribe(context.Background(), audio.SpeechFormat, pcm)
	if err != nil {
		t.Fatalf("Transcribe: %v", err)
	}
	if res.Text != "¿Qué hora es?" {
		t.Errorf("text %q", res.Text)
	}
	if res.Processing <= 0 || res.Total < res.Processing {
		t.Errorf("processing %v, total %v", res.Processing, res.Total)
	}

	req := <-seen
	if req.language != "es" {
		t.Errorf("language %q, want es", req.language)
	}
	if req.format != (wyoming.AudioFormat{Rate: 16000, Width: 2, Channels: 1}) {
		t.Errorf("format %+v", req.format)
	}
	if !bytes.Equal(req.audio, pcm) {
		t.Errorf("service got %d bytes, want the %d bytes sent", len(req.audio), len(pcm))
	}
}

func TestTranscribeEmptyText(t *testing.T) {
	addr, _ := fakeWhisper(t, func(conn net.Conn) {
		send(conn, wyoming.TypeTranscript, wyoming.Transcript{Text: "  "})
	})
	res, err := newClient(addr).Transcribe(context.Background(), audio.SpeechFormat, make([]byte, 3200))
	if err != nil || res.Text != "" {
		t.Fatalf("got %q, %v; want empty text and no error", res.Text, err)
	}
}

func TestTranscribeServiceError(t *testing.T) {
	addr, _ := fakeWhisper(t, func(conn net.Conn) {
		send(conn, wyoming.TypeError, wyoming.Error{Text: "model failed"})
	})
	_, err := newClient(addr).Transcribe(context.Background(), audio.SpeechFormat, make([]byte, 3200))
	var se *wyoming.ServiceError
	if !errors.As(err, &se) {
		t.Fatalf("error %v, want a ServiceError", err)
	}
}

func TestTranscribeConnectionClosed(t *testing.T) {
	addr, _ := fakeWhisper(t, func(net.Conn) {})
	_, err := newClient(addr).Transcribe(context.Background(), audio.SpeechFormat, make([]byte, 3200))
	if !errors.Is(err, wyoming.ErrConnectionClosed) {
		t.Fatalf("error %v, want ErrConnectionClosed", err)
	}
}

func TestTranscribeTimeout(t *testing.T) {
	addr, _ := fakeWhisper(t, func(conn net.Conn) { time.Sleep(time.Second) })
	c := New(Config{Addr: addr, Language: "es", Timeouts: wyoming.Timeouts{Connect: time.Second, Operation: 50 * time.Millisecond}})
	start := time.Now()
	if _, err := c.Transcribe(context.Background(), audio.SpeechFormat, make([]byte, 3200)); err == nil {
		t.Fatal("no error from a service that does not answer")
	}
	if time.Since(start) > 500*time.Millisecond {
		t.Errorf("Transcribe took %v with a 50ms operation timeout", time.Since(start))
	}
}

func TestTranscribeServiceUnavailable(t *testing.T) {
	ln, _ := net.Listen("tcp", "127.0.0.1:0")
	addr := ln.Addr().String()
	ln.Close()
	if _, err := newClient(addr).Transcribe(context.Background(), audio.SpeechFormat, make([]byte, 3200)); err == nil {
		t.Fatal("no error from a closed port")
	}
}
