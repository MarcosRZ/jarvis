package tailrec

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"jarvis/client/internal/audio"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestWriteWAV(t *testing.T) {
	path := filepath.Join(t.TempDir(), "tail.wav")
	pcm := []byte{0x01, 0x02, 0x03, 0x04}
	if err := writeWAV(path, audio.SpeechFormat, pcm); err != nil {
		t.Fatalf("writeWAV: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	if len(data) != 44+len(pcm) {
		t.Fatalf("size %d, want %d", len(data), 44+len(pcm))
	}
	if got := string(data[0:4]); got != "RIFF" {
		t.Errorf("RIFF %q", got)
	}
	if got := binary.LittleEndian.Uint32(data[4:]); got != uint32(36+len(pcm)) {
		t.Errorf("RIFF size %d", got)
	}
	if got := string(data[8:12]); got != "WAVE" {
		t.Errorf("WAVE %q", got)
	}
	if got := binary.LittleEndian.Uint16(data[20:]); got != 1 {
		t.Errorf("format %d, want PCM", got)
	}
	if got := binary.LittleEndian.Uint16(data[22:]); got != uint16(audio.SpeechFormat.Channels) {
		t.Errorf("channels %d", got)
	}
	if got := binary.LittleEndian.Uint32(data[24:]); got != uint32(audio.SpeechFormat.SampleRate) {
		t.Errorf("rate %d", got)
	}
	if got := binary.LittleEndian.Uint16(data[34:]); got != 16 {
		t.Errorf("bits %d, want 16", got)
	}
	if got := binary.LittleEndian.Uint32(data[40:]); got != uint32(len(pcm)) {
		t.Errorf("data size %d", got)
	}
	if !bytes.Equal(data[44:], pcm) {
		t.Errorf("PCM %x, want %x", data[44:], pcm)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("mode %o, want 600", got)
	}
}

func TestPruneKeepsNewest(t *testing.T) {
	dir := t.TempDir()
	for i := range 12 {
		name := fmt.Sprintf("tail-20260101-0000%02d.000000000.wav", i)
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o600); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	// Other names are not ours and stay.
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	r := &Recorder{dir: dir, keep: 10}
	if err := r.prune(); err != nil {
		t.Fatalf("prune: %v", err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var tails []string
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), filePrefix) {
			tails = append(tails, entry.Name())
		}
	}
	if len(tails) != 10 {
		t.Fatalf("%d tails, want 10: %v", len(tails), tails)
	}
	if _, err := os.Stat(filepath.Join(dir, "tail-20260101-000000.000000000.wav")); !os.IsNotExist(err) {
		t.Errorf("oldest tail still exists")
	}
	if _, err := os.Stat(filepath.Join(dir, "tail-20260101-000011.000000000.wav")); err != nil {
		t.Errorf("newest tail missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "notes.txt")); err != nil {
		t.Errorf("unrelated file removed: %v", err)
	}
}

type fakeSource struct {
	pcm []byte
}

func (f fakeSource) Tail(context.Context, time.Duration) ([]byte, error) {
	return f.pcm, nil
}

func TestStartSavesOneTail(t *testing.T) {
	dir := t.TempDir()
	pcm := []byte{1, 0, 2, 0, 3, 0, 4, 0}
	r := New(dir, 10, 3*time.Second, audio.SpeechFormat, fakeSource{pcm: pcm}, testLogger())
	r.Start(context.Background())

	var path string
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		if len(entries) == 1 {
			path = filepath.Join(dir, entries[0].Name())
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if path == "" {
		t.Fatal("tail file was not created")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(data[44:], pcm) {
		t.Errorf("PCM %x, want %x", data[44:], pcm)
	}
}

type blockingSource struct {
	started chan struct{}
	release chan struct{}
}

func (b blockingSource) Tail(ctx context.Context, _ time.Duration) ([]byte, error) {
	close(b.started)
	select {
	case <-b.release:
		return nil, errors.New("stop")
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func TestStartSkipsWhileBusy(t *testing.T) {
	dir := t.TempDir()
	src := blockingSource{started: make(chan struct{}), release: make(chan struct{})}
	r := New(dir, 10, 3*time.Second, audio.SpeechFormat, src, testLogger())
	r.Start(context.Background())
	<-src.started
	r.Start(context.Background())
	close(src.release)

	deadline := time.Now().Add(2 * time.Second)
	for r.running.Load() && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 0 {
		t.Fatalf("entries %d, err %v; want none", len(entries), err)
	}
}
