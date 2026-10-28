// Package learning stores sentence-to-action associations and their feedback,
// so a sentence that the model had to infer becomes a fast, local lookup.
package learning

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"
)

// Entry is one learned sentence. Text is normalized by the caller, and Kind is
// the command kind ("turn-on" or "turn-off").
type Entry struct {
	Text      string    `json:"text"`
	Device    string    `json:"device"`
	Name      string    `json:"name,omitempty"`
	Kind      string    `json:"kind"`
	Score     int       `json:"score"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Updated   time.Time `json:"updated"`
}

// Store keeps the entries in memory and writes them to a JSON file after each
// change. It is safe for concurrent use.
type Store struct {
	path    string
	mu      sync.Mutex
	entries map[string]Entry
}

type file struct {
	Entries []Entry `json:"entries"`
}

// Open reads path, or starts empty when the file does not exist. An empty path
// is an error: the caller uses it to disable learning.
func Open(path string) (*Store, error) {
	if path == "" {
		return nil, errors.New("empty learning file")
	}
	s := &Store{path: path, entries: map[string]Entry{}}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read learning file: %w", err)
	}
	if len(b) == 0 {
		return s, nil
	}
	var f file
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, fmt.Errorf("parse learning file %s: %w", path, err)
	}
	for _, e := range f.Entries {
		if e.Text != "" {
			s.entries[e.Text] = e
		}
	}
	return s, nil
}

// Len returns the number of learned sentences.
func (s *Store) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Lookup returns the entry for text, if any. A negative score means the user
// punished that action; the caller should not run it again.
func (s *Store) Lookup(text string) (Entry, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[text]
	return e, ok
}

// Learn records that text ran device/kind successfully. The same action adds
// one success and one point; a different action replaces the association and
// starts again at one point.
func (s *Store) Learn(text, device, name, kind string, now time.Time) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[text]
	if !ok || e.Device != device || e.Kind != kind {
		e = Entry{Text: text, Device: device, Name: name, Kind: kind, Score: 1, Successes: 1}
	} else {
		e.Score++
		e.Successes++
	}
	e.Updated = now
	s.entries[text] = e
	return e, s.saveLocked()
}

// Penalize records that the last run of text was wrong. It undoes the success
// counted at execution time and sets the score to -1, so even a long success
// streak cannot keep a punished action in use.
func (s *Store) Penalize(text, device, kind string, now time.Time) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[text]
	if !ok || e.Device != device || e.Kind != kind {
		return Entry{}, false, nil
	}
	e.Failures++
	if e.Successes > 0 {
		e.Successes--
	}
	e.Score = -1
	e.Updated = now
	s.entries[text] = e
	return e, true, s.saveLocked()
}

func (s *Store) saveLocked() error {
	entries := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Text < entries[j].Text })
	b, err := json.MarshalIndent(file{Entries: entries}, "", "  ")
	if err != nil {
		return fmt.Errorf("encode learning file: %w", err)
	}
	dir := filepath.Dir(s.path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create learning directory: %w", err)
	}
	tmp, err := os.CreateTemp(dir, ".learning-*.json")
	if err != nil {
		return fmt.Errorf("create temporary learning file: %w", err)
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.Write(b); err != nil {
		tmp.Close()
		return fmt.Errorf("write learning file: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close learning file: %w", err)
	}
	if err := os.Rename(name, s.path); err != nil {
		return fmt.Errorf("replace learning file: %w", err)
	}
	return nil
}
