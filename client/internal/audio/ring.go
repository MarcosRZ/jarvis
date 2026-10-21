package audio

import (
	"context"
	"sync"
)

// Ring is a fixed-size FIFO of PCM bytes between an audio callback and Go code.
// Push and Pop never block, so the audio callback can use them. Read, Write and
// WaitEmpty block until they can progress, the context ends or the ring closes.
// All sizes are whole frames, so a sample is never split.
type Ring struct {
	mu        sync.Mutex
	buf       []byte
	frameSize int
	start     int
	length    int
	closeErr  error
	readable  chan struct{}
	writable  chan struct{}
}

func NewRing(capacity, frameSize int) *Ring {
	if capacity < frameSize {
		panic("audio: ring capacity is smaller than one frame")
	}
	return &Ring{
		buf:       make([]byte, capacity/frameSize*frameSize),
		frameSize: frameSize,
		readable:  make(chan struct{}, 1),
		writable:  make(chan struct{}, 1),
	}
}

func (r *Ring) Cap() int { return len(r.buf) }

func (r *Ring) Len() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.length
}

// Push adds p and drops the oldest bytes when the ring is full. It returns the
// number of dropped bytes.
func (r *Ring) Push(p []byte) int {
	r.mu.Lock()
	dropped := 0
	if len(p) > len(r.buf) {
		dropped = len(p) - len(r.buf)
		p = p[dropped:]
	}
	if over := r.length + len(p) - len(r.buf); over > 0 {
		r.discard(over)
		dropped += over
	}
	r.put(p)
	r.mu.Unlock()
	signal(r.readable)
	return dropped
}

// Pop moves up to len(dst) bytes, in whole frames, into dst.
func (r *Ring) Pop(dst []byte) int {
	r.mu.Lock()
	n := r.take(dst)
	r.mu.Unlock()
	if n > 0 {
		signal(r.writable)
	}
	return n
}

// Read waits for data and then moves up to len(dst) bytes into dst. After
// Close it returns the buffered data first and then the close error.
func (r *Ring) Read(ctx context.Context, dst []byte) (int, error) {
	for {
		r.mu.Lock()
		n := r.take(dst)
		closeErr := r.closeErr
		r.mu.Unlock()
		if n > 0 {
			signal(r.writable)
			return n, nil
		}
		if closeErr != nil {
			return 0, closeErr
		}
		select {
		case <-r.readable:
		case <-ctx.Done():
			return 0, ctx.Err()
		}
	}
}

// Write adds all of p, and waits for free space when the ring is full.
func (r *Ring) Write(ctx context.Context, p []byte) error {
	for {
		r.mu.Lock()
		if r.closeErr != nil {
			r.mu.Unlock()
			return r.closeErr
		}
		n := min(len(p), len(r.buf)-r.length) / r.frameSize * r.frameSize
		r.put(p[:n])
		r.mu.Unlock()
		if n > 0 {
			signal(r.readable)
		}
		p = p[n:]
		if len(p) < r.frameSize {
			return nil
		}
		select {
		case <-r.writable:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// WaitEmpty waits until all buffered data has been popped.
func (r *Ring) WaitEmpty(ctx context.Context) error {
	for {
		r.mu.Lock()
		length, closeErr := r.length, r.closeErr
		r.mu.Unlock()
		if length == 0 {
			return nil
		}
		if closeErr != nil {
			return closeErr
		}
		select {
		case <-r.writable:
		case <-ctx.Done():
			return ctx.Err()
		}
	}
}

// Reset drops all buffered data.
func (r *Ring) Reset() {
	r.mu.Lock()
	r.start, r.length = 0, 0
	r.mu.Unlock()
	signal(r.writable)
}

// Close makes blocked and later calls return err once the buffer is empty.
func (r *Ring) Close(err error) {
	r.mu.Lock()
	if r.closeErr == nil {
		r.closeErr = err
	}
	r.mu.Unlock()
	signal(r.readable)
	signal(r.writable)
}

func (r *Ring) put(p []byte) {
	end := (r.start + r.length) % len(r.buf)
	n := copy(r.buf[end:], p)
	copy(r.buf, p[n:])
	r.length += len(p)
}

func (r *Ring) take(dst []byte) int {
	n := min(len(dst), r.length) / r.frameSize * r.frameSize
	first := copy(dst[:n], r.buf[r.start:])
	copy(dst[first:n], r.buf)
	r.discard(n)
	return n
}

func (r *Ring) discard(n int) {
	r.start = (r.start + n) % len(r.buf)
	r.length -= n
}

func signal(ch chan struct{}) {
	select {
	case ch <- struct{}{}:
	default:
	}
}
