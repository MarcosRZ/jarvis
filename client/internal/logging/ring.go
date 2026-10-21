package logging

import (
	"bytes"
	"sync"
)

// Line is one log line with its number; numbers start at 1 and only grow, so a
// reader can ask for the lines after the last one it saw.
type Line struct {
	Seq  uint64
	Text string
}

// Ring keeps the last lines written to it in memory, for the log web page.
// Nothing goes to disk, and the lines are gone when the client stops. It is
// safe for concurrent use.
type Ring struct {
	mu      sync.Mutex
	lines   []Line
	max     int
	last    uint64
	partial []byte
}

func NewRing(max int) *Ring {
	return &Ring{max: max}
}

// Write stores each complete line of p. slog writes one record per Write.
func (r *Ring) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	data := append(r.partial, p...)
	for {
		i := bytes.IndexByte(data, '\n')
		if i < 0 {
			break
		}
		r.last++
		r.lines = append(r.lines, Line{Seq: r.last, Text: string(data[:i])})
		if len(r.lines) > r.max {
			r.lines = r.lines[len(r.lines)-r.max:]
		}
		data = data[i+1:]
	}
	r.partial = bytes.Clone(data)
	return len(p), nil
}

// After returns the kept lines with a number greater than seq, at most limit
// of them (the last ones), and the number of the last line written.
func (r *Ring) After(seq uint64, limit int) ([]Line, uint64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	var out []Line
	for _, l := range r.lines {
		if l.Seq > seq {
			out = append(out, l)
		}
	}
	if limit > 0 && len(out) > limit {
		out = out[len(out)-limit:]
	}
	return out, r.last
}
