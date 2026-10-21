package wyoming

import (
	"bufio"
	"context"
	"errors"
	"net"
	"os"
	"strings"
	"testing"
	"time"
)

var testTimeouts = Timeouts{Connect: time.Second, Operation: 2 * time.Second}

// serve starts a fake service that runs handle for one connection.
func serve(t *testing.T, handle func(net.Conn)) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		handle(conn)
	}()
	return ln.Addr().String()
}

func waitForClose(conn net.Conn) {
	_, _ = conn.Read(make([]byte, 1))
}

func dial(t *testing.T, addr string, timeouts Timeouts) *Conn {
	t.Helper()
	c, err := Dial(context.Background(), addr, timeouts)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestConnRoundTrip(t *testing.T) {
	addr := serve(t, func(conn net.Conn) {
		ev, err := ReadEvent(bufio.NewReader(conn))
		if err != nil || ev.Type != TypeTranscribe {
			return
		}
		reply, _ := NewEvent(TypeTranscript, Transcript{Text: "hola"}, nil)
		_ = WriteEvent(conn, reply)
		waitForClose(conn)
	})
	c := dial(t, addr, testTimeouts)

	request, _ := NewEvent(TypeTranscribe, Transcribe{Language: "es"}, nil)
	if err := c.Write(context.Background(), request); err != nil {
		t.Fatalf("Write: %v", err)
	}
	ev, err := c.Read(context.Background())
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	var tr Transcript
	if err := ev.Decode(&tr); err != nil || tr.Text != "hola" {
		t.Fatalf("got %+v, %v", tr, err)
	}
}

func TestConnReadStopsOnCancel(t *testing.T) {
	addr := serve(t, waitForClose)
	c := dial(t, addr, testTimeouts)
	ctx, cancel := context.WithCancel(context.Background())
	time.AfterFunc(20*time.Millisecond, cancel)

	start := time.Now()
	_, err := c.Read(ctx)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
	if time.Since(start) > time.Second {
		t.Fatalf("Read took %v after cancel", time.Since(start))
	}
}

func TestConnReadOperationTimeout(t *testing.T) {
	addr := serve(t, waitForClose)
	c := dial(t, addr, Timeouts{Connect: time.Second, Operation: 30 * time.Millisecond})

	_, err := c.Read(context.Background())
	if !errors.Is(err, os.ErrDeadlineExceeded) {
		t.Fatalf("error %v, want deadline exceeded", err)
	}
	if !strings.Contains(err.Error(), addr) {
		t.Errorf("error %q does not name %s", err, addr)
	}
}

func TestConnReadReportsClosedConnection(t *testing.T) {
	addr := serve(t, func(net.Conn) {})
	c := dial(t, addr, testTimeouts)
	if _, err := c.Read(context.Background()); !errors.Is(err, ErrConnectionClosed) {
		t.Fatalf("error %v, want ErrConnectionClosed", err)
	}
}

func TestConnReadReportsMalformedEvent(t *testing.T) {
	addr := serve(t, func(conn net.Conn) {
		_, _ = conn.Write([]byte("garbage\n"))
		waitForClose(conn)
	})
	c := dial(t, addr, testTimeouts)
	if _, err := c.Read(context.Background()); !errors.Is(err, ErrMalformed) {
		t.Fatalf("error %v, want ErrMalformed", err)
	}
}

func TestConnWriteAfterCloseFails(t *testing.T) {
	addr := serve(t, waitForClose)
	c := dial(t, addr, testTimeouts)
	c.Close()
	ev, _ := NewEvent(TypeDescribe, nil, nil)
	if err := c.Write(context.Background(), ev); !errors.Is(err, net.ErrClosed) {
		t.Fatalf("error %v, want net.ErrClosed", err)
	}
}

func TestDialRefused(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()

	_, err = Dial(context.Background(), addr, testTimeouts)
	if err == nil || !strings.Contains(err.Error(), addr) {
		t.Fatalf("error %v, want a dial error that names %s", err, addr)
	}
}

func TestDialStopsOnCancelledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Dial(ctx, "127.0.0.1:1", testTimeouts); !errors.Is(err, context.Canceled) {
		t.Fatalf("error %v, want context.Canceled", err)
	}
}

func TestDescribe(t *testing.T) {
	addr := serve(t, func(conn net.Conn) {
		ev, err := ReadEvent(bufio.NewReader(conn))
		if err != nil || ev.Type != TypeDescribe {
			return
		}
		info, _ := NewEvent(TypeInfo, Info{Wake: []Program{{Name: "openwakeword", Models: []Model{{Name: "hey_jarvis", Languages: []string{"en"}}}}}}, nil)
		_ = WriteEvent(conn, info)
	})

	info, err := Describe(context.Background(), addr, testTimeouts)
	if err != nil {
		t.Fatalf("Describe: %v", err)
	}
	if len(info.Wake) != 1 || info.Wake[0].Models[0].Name != "hey_jarvis" {
		t.Fatalf("info %+v", info)
	}
}

func TestDescribeReportsServiceError(t *testing.T) {
	addr := serve(t, func(conn net.Conn) {
		if _, err := ReadEvent(bufio.NewReader(conn)); err != nil {
			return
		}
		reply, _ := NewEvent(TypeError, Error{Text: "busy"}, nil)
		_ = WriteEvent(conn, reply)
	})
	var se *ServiceError
	if _, err := Describe(context.Background(), addr, testTimeouts); !errors.As(err, &se) {
		t.Fatalf("error %v, want ServiceError", err)
	}
}
