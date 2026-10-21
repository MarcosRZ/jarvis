package audio

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"
)

const frame = 2

func TestRingCapacityIsWholeFrames(t *testing.T) {
	r := NewRing(7, frame)
	if got := r.Cap(); got != 6 {
		t.Fatalf("Cap() = %d, want 6", got)
	}
}

func TestRingPushPopKeepsOrderAcrossWrap(t *testing.T) {
	r := NewRing(6, frame)
	r.Push([]byte{1, 2, 3, 4})
	dst := make([]byte, 2)
	r.Pop(dst)
	r.Push([]byte{5, 6, 7, 8})

	dst = make([]byte, 6)
	n := r.Pop(dst)
	if want := []byte{3, 4, 5, 6, 7, 8}; !bytes.Equal(dst[:n], want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
	if r.Len() != 0 {
		t.Fatalf("Len() = %d after full pop", r.Len())
	}
}

func TestRingPushDropsOldestWholeFrames(t *testing.T) {
	r := NewRing(4, frame)
	if dropped := r.Push([]byte{1, 2, 3, 4}); dropped != 0 {
		t.Fatalf("dropped %d with free space", dropped)
	}
	if dropped := r.Push([]byte{5, 6}); dropped != 2 {
		t.Fatalf("dropped %d, want 2", dropped)
	}
	dst := make([]byte, 4)
	n := r.Pop(dst)
	if want := []byte{3, 4, 5, 6}; !bytes.Equal(dst[:n], want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}

func TestRingPushLargerThanCapacityKeepsNewest(t *testing.T) {
	r := NewRing(4, frame)
	r.Push([]byte{9, 9})
	dropped := r.Push([]byte{1, 2, 3, 4, 5, 6})
	if dropped != 4 {
		t.Fatalf("dropped %d, want 4", dropped)
	}
	dst := make([]byte, 4)
	n := r.Pop(dst)
	if want := []byte{3, 4, 5, 6}; !bytes.Equal(dst[:n], want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}

func TestRingPopReturnsWholeFramesOnly(t *testing.T) {
	r := NewRing(8, frame)
	r.Push([]byte{1, 2, 3, 4})
	dst := make([]byte, 3)
	if n := r.Pop(dst); n != 2 {
		t.Fatalf("Pop into 3 bytes returned %d, want 2", n)
	}
}

func TestRingReadWaitsForData(t *testing.T) {
	r := NewRing(8, frame)
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Push([]byte{1, 2})
	}()
	dst := make([]byte, 8)
	n, err := r.Read(context.Background(), dst)
	if err != nil || n != 2 {
		t.Fatalf("Read = %d, %v; want 2, nil", n, err)
	}
}

func TestRingReadStopsOnCancel(t *testing.T) {
	r := NewRing(8, frame)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := r.Read(ctx, make([]byte, 2)); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Read error %v, want deadline exceeded", err)
	}
}

func TestRingReadDrainsThenReportsClose(t *testing.T) {
	r := NewRing(8, frame)
	r.Push([]byte{1, 2})
	cause := errors.New("device gone")
	r.Close(cause)

	dst := make([]byte, 8)
	if n, err := r.Read(context.Background(), dst); n != 2 || err != nil {
		t.Fatalf("first Read = %d, %v; want buffered data", n, err)
	}
	if _, err := r.Read(context.Background(), dst); !errors.Is(err, cause) {
		t.Fatalf("second Read error %v, want %v", err, cause)
	}
}

func TestRingCloseReleasesBlockedRead(t *testing.T) {
	r := NewRing(8, frame)
	cause := errors.New("device gone")
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Close(cause)
	}()
	if _, err := r.Read(context.Background(), make([]byte, 2)); !errors.Is(err, cause) {
		t.Fatalf("Read error %v, want %v", err, cause)
	}
}

func TestRingWriteWaitsForSpace(t *testing.T) {
	r := NewRing(4, frame)
	go func() {
		for range 3 {
			time.Sleep(10 * time.Millisecond)
			r.Pop(make([]byte, 2))
		}
	}()
	if err := r.Write(context.Background(), []byte{1, 2, 3, 4, 5, 6, 7, 8}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	dst := make([]byte, 4)
	n := r.Pop(dst)
	if want := []byte{5, 6, 7, 8}; !bytes.Equal(dst[:n], want) {
		t.Fatalf("got %v, want %v", dst[:n], want)
	}
}

func TestRingWriteStopsOnCancel(t *testing.T) {
	r := NewRing(2, frame)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if err := r.Write(ctx, []byte{1, 2, 3, 4}); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Write error %v, want deadline exceeded", err)
	}
}

func TestRingWriteAfterCloseFails(t *testing.T) {
	r := NewRing(4, frame)
	cause := errors.New("device gone")
	r.Close(cause)
	if err := r.Write(context.Background(), []byte{1, 2}); !errors.Is(err, cause) {
		t.Fatalf("Write error %v, want %v", err, cause)
	}
}

func TestRingReset(t *testing.T) {
	r := NewRing(6, frame)
	r.Push([]byte{1, 2, 3, 4})
	r.Pop(make([]byte, 2))
	r.Reset()
	if r.Len() != 0 {
		t.Fatalf("Len() = %d after Reset", r.Len())
	}
	r.Push([]byte{7, 8})
	dst := make([]byte, 6)
	if n := r.Pop(dst); !bytes.Equal(dst[:n], []byte{7, 8}) {
		t.Fatalf("got %v after Reset and Push, want [7 8]", dst[:n])
	}
}

func TestRingWaitEmpty(t *testing.T) {
	r := NewRing(4, frame)
	r.Push([]byte{1, 2})
	go func() {
		time.Sleep(20 * time.Millisecond)
		r.Pop(make([]byte, 2))
	}()
	if err := r.WaitEmpty(context.Background()); err != nil {
		t.Fatalf("WaitEmpty: %v", err)
	}
}
