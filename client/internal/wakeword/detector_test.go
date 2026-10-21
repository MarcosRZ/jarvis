package wakeword

import (
	"bufio"
	"context"
	"errors"
	"io"
	"log/slog"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

const testModel = "hey_jarvis"

var testTimeouts = wyoming.Timeouts{Connect: time.Second, Operation: time.Second}

// streamFunc handles one audio stream connection; n is 0 for the first one.
type streamFunc func(n int, conn net.Conn, r *bufio.Reader)

type fakeWake struct {
	models        []string
	failDescribes int
	stream        streamFunc

	mu      sync.Mutex
	streams int
}

func (f *fakeWake) start(t *testing.T) string {
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
			go f.handle(conn)
		}
	}()
	return ln.Addr().String()
}

func (f *fakeWake) handle(conn net.Conn) {
	defer conn.Close()
	r := bufio.NewReader(conn)
	ev, err := wyoming.ReadEvent(r)
	if err != nil {
		return
	}
	if ev.Type == wyoming.TypeDescribe {
		f.mu.Lock()
		fail := f.failDescribes > 0
		f.failDescribes--
		f.mu.Unlock()
		if fail {
			reply, _ := wyoming.NewEvent(wyoming.TypeError, wyoming.Error{Text: "starting"}, nil)
			_ = wyoming.WriteEvent(conn, reply)
			return
		}
		models := make([]wyoming.Model, len(f.models))
		for i, name := range f.models {
			models[i] = wyoming.Model{Name: name}
		}
		reply, _ := wyoming.NewEvent(wyoming.TypeInfo, wyoming.Info{Wake: []wyoming.Program{{Name: "openwakeword", Models: models}}}, nil)
		_ = wyoming.WriteEvent(conn, reply)
		return
	}
	f.mu.Lock()
	n := f.streams
	f.streams++
	f.mu.Unlock()
	f.stream(n, conn, prepend(ev, r))
}

// prepend returns a reader that gives ev again before the rest of r, so a stream
// handler sees the detect event.
func prepend(ev wyoming.Event, r *bufio.Reader) *bufio.Reader {
	pr, pw := io.Pipe()
	go func() {
		_ = wyoming.WriteEvent(pw, ev)
		_, err := io.Copy(pw, r)
		pw.CloseWithError(err)
	}()
	return bufio.NewReader(pr)
}

// readChunks reads events until n audio chunks have arrived.
func readChunks(r *bufio.Reader, n int) bool {
	for n > 0 {
		ev, err := wyoming.ReadEvent(r)
		if err != nil {
			return false
		}
		if ev.Type == wyoming.TypeAudioChunk {
			n--
		}
	}
	return true
}

func detect(conn net.Conn, timestampMs int64) {
	ev, _ := wyoming.NewEvent(wyoming.TypeDetection, wyoming.Detection{Name: testModel, Timestamp: timestampMs}, nil)
	_ = wyoming.WriteEvent(conn, ev)
}

func drain(r *bufio.Reader) {
	for {
		if _, err := wyoming.ReadEvent(r); err != nil {
			return
		}
	}
}

// silence is a Source of zero samples at a fast pace. When failAfter > 0, Read
// returns err after that number of reads.
type silence struct {
	failAfter int
	err       error
	reads     int
}

func (s *silence) Read(ctx context.Context, p []byte) (int, error) {
	s.reads++
	if s.failAfter > 0 && s.reads > s.failAfter {
		return 0, s.err
	}
	select {
	case <-time.After(time.Millisecond):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	clear(p)
	return len(p), nil
}

func newDetector(addr string, cooldown time.Duration) *Detector {
	d := New(Config{Addr: addr, Model: testModel, Cooldown: cooldown, Timeouts: testTimeouts, Logger: slog.New(slog.DiscardHandler)}, audio.SpeechFormat)
	d.retryMin = 10 * time.Millisecond
	d.retryMax = 20 * time.Millisecond
	return d
}

type runResult struct {
	detections []Detection
	err        error
}

// run runs d until want detections arrive, Run returns, or the timeout.
func run(t *testing.T, d *Detector, src audio.Source, want int, timeout time.Duration) runResult {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()
	out := make(chan Detection)
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, src, out) }()

	var res runResult
	for {
		select {
		case det := <-out:
			res.detections = append(res.detections, det)
			if len(res.detections) == want {
				cancel()
				<-done
				return res
			}
		case res.err = <-done:
			return res
		}
	}
}

func TestRunRejectsUnknownModel(t *testing.T) {
	addr := (&fakeWake{models: []string{"okay_nabu", "alexa"}}).start(t)
	res := run(t, newDetector(addr, 0), &silence{}, 1, 2*time.Second)
	if !errors.Is(res.err, ErrUnknownModel) {
		t.Fatalf("error %v, want ErrUnknownModel", res.err)
	}
	if !strings.Contains(res.err.Error(), "okay_nabu") {
		t.Errorf("error %q does not list the available models", res.err)
	}
}

func TestRunStreamsAudioAndReportsDetection(t *testing.T) {
	type observed struct {
		names  []string
		format wyoming.AudioFormat
		sizes  []int
	}
	seen := make(chan observed, 1)
	addr := (&fakeWake{models: []string{testModel}, stream: func(_ int, conn net.Conn, r *bufio.Reader) {
		var o observed
		for ev, err := wyoming.ReadEvent(r); err == nil; ev, err = wyoming.ReadEvent(r) {
			switch ev.Type {
			case wyoming.TypeDetect:
				var d wyoming.Detect
				_ = ev.Decode(&d)
				o.names = d.Names
			case wyoming.TypeAudioStart:
				_ = ev.Decode(&o.format)
			case wyoming.TypeAudioChunk:
				o.sizes = append(o.sizes, len(ev.Payload))
			}
			if len(o.sizes) == 5 {
				seen <- o
				detect(conn, 320)
				drain(r)
				return
			}
		}
	}}).start(t)

	res := run(t, newDetector(addr, 0), &silence{}, 1, 3*time.Second)
	if len(res.detections) != 1 {
		t.Fatalf("got %d detections, error %v", len(res.detections), res.err)
	}
	det := res.detections[0]
	if det.Model != testModel || det.StreamOffset != 320*time.Millisecond {
		t.Errorf("detection %+v", det)
	}
	if det.Latency <= 0 || det.Latency > time.Second {
		t.Errorf("latency %v, want the time since chunk 4 was sent", det.Latency)
	}

	o := <-seen
	if !slices.Equal(o.names, []string{testModel}) {
		t.Errorf("detect names %v", o.names)
	}
	if o.format != (wyoming.AudioFormat{Rate: 16000, Width: 2, Channels: 1}) {
		t.Errorf("audio-start format %+v", o.format)
	}
	if slices.ContainsFunc(o.sizes, func(n int) bool { return n != 2560 }) {
		t.Errorf("chunk sizes %v, want 80 ms (2560 bytes) each", o.sizes)
	}
}

func TestRunCooldownIgnoresRepeatedDetections(t *testing.T) {
	stream := func(_ int, conn net.Conn, r *bufio.Reader) {
		if !readChunks(r, 3) {
			return
		}
		detect(conn, 80)
		detect(conn, 160)
		drain(r)
	}
	tests := []struct {
		cooldown time.Duration
		want     int
	}{{time.Minute, 1}, {0, 2}}
	for _, tt := range tests {
		addr := (&fakeWake{models: []string{testModel}, stream: stream}).start(t)
		res := run(t, newDetector(addr, tt.cooldown), &silence{}, 2, 500*time.Millisecond)
		if len(res.detections) != tt.want {
			t.Errorf("cooldown %v: got %d detections, want %d", tt.cooldown, len(res.detections), tt.want)
		}
	}
}

func TestRunReconnectsAfterConnectionLoss(t *testing.T) {
	addr := (&fakeWake{models: []string{testModel}, stream: func(n int, conn net.Conn, r *bufio.Reader) {
		if !readChunks(r, 2) {
			return
		}
		if n == 0 {
			return
		}
		detect(conn, 80)
		drain(r)
	}}).start(t)

	res := run(t, newDetector(addr, 0), &silence{}, 1, 3*time.Second)
	if len(res.detections) != 1 {
		t.Fatalf("got %d detections after a lost connection, error %v", len(res.detections), res.err)
	}
}

func TestRunRetriesWhileServiceIsNotReady(t *testing.T) {
	addr := (&fakeWake{models: []string{testModel}, failDescribes: 2, stream: func(_ int, conn net.Conn, r *bufio.Reader) {
		if readChunks(r, 1) {
			detect(conn, 0)
			drain(r)
		}
	}}).start(t)

	res := run(t, newDetector(addr, 0), &silence{}, 1, 3*time.Second)
	if len(res.detections) != 1 {
		t.Fatalf("got %d detections, error %v", len(res.detections), res.err)
	}
}

func TestRunReturnsSourceError(t *testing.T) {
	addr := (&fakeWake{models: []string{testModel}, stream: func(_ int, _ net.Conn, r *bufio.Reader) { drain(r) }}).start(t)
	micGone := errors.New("mic gone")

	res := run(t, newDetector(addr, 0), &silence{failAfter: 3, err: micGone}, 1, 3*time.Second)
	if !errors.Is(res.err, micGone) {
		t.Fatalf("error %v, want the source error", res.err)
	}
}

// countingSilence is silence that counts its reads safely across goroutines.
type countingSilence struct {
	mu    sync.Mutex
	reads int
}

func (s *countingSilence) Read(ctx context.Context, p []byte) (int, error) {
	s.mu.Lock()
	s.reads++
	s.mu.Unlock()
	select {
	case <-time.After(time.Millisecond):
	case <-ctx.Done():
		return 0, ctx.Err()
	}
	clear(p)
	return len(p), nil
}

func (s *countingSilence) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.reads
}

func TestWaitReturnsFirstDetectionAndStopsReading(t *testing.T) {
	addr := (&fakeWake{models: []string{testModel}, stream: func(_ int, conn net.Conn, r *bufio.Reader) {
		if readChunks(r, 2) {
			detect(conn, 80)
			drain(r)
		}
	}}).start(t)
	src := &countingSilence{}

	det, err := newDetector(addr, 0).Wait(context.Background(), src)
	if err != nil || det.Model != testModel {
		t.Fatalf("Wait = %+v, %v", det, err)
	}
	reads := src.count()
	time.Sleep(50 * time.Millisecond)
	if src.count() != reads {
		t.Fatalf("the detector read the source %d more times after Wait returned", src.count()-reads)
	}
}

func TestWaitReturnsRunError(t *testing.T) {
	addr := (&fakeWake{models: []string{"okay_nabu"}}).start(t)
	if _, err := newDetector(addr, 0).Wait(context.Background(), &silence{}); !errors.Is(err, ErrUnknownModel) {
		t.Fatalf("error %v, want ErrUnknownModel", err)
	}
}

func TestRunStopsOnCancel(t *testing.T) {
	addr := (&fakeWake{models: []string{testModel}, stream: func(_ int, _ net.Conn, r *bufio.Reader) { drain(r) }}).start(t)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- newDetector(addr, 0).Run(ctx, &silence{}, make(chan Detection)) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop after cancel")
	}
}
