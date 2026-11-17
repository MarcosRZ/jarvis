// Package learning stores sentence-to-action associations and their feedback,
// so a sentence that the model had to infer becomes a fast, local lookup.
package learning

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
)

// SnapshotVersion is the schema version of the snapshot published to the
// dashboard.
const SnapshotVersion = 1

// Entry is one learned sentence. Text is normalized by the caller. An entry
// names either a device and its Kind ("turn-on" or "turn-off") or one action of
// the generated catalog, for example "scene.cine.turn_on".
type Entry struct {
	Text      string    `json:"text"`
	Device    string    `json:"device,omitempty"`
	Name      string    `json:"name,omitempty"`
	Kind      string    `json:"kind,omitempty"`
	Action    string    `json:"action,omitempty"`
	Score     int       `json:"score"`
	Successes int       `json:"successes"`
	Failures  int       `json:"failures"`
	Updated   time.Time `json:"updated"`
}

// Snapshot is the read-only form of the store that the client publishes to the
// dashboard. Text is recognized speech, so a snapshot is private data.
// AppliedThrough acknowledges the dashboard operations that the client has
// already applied, so the dashboard can drop them from its queue.
type Snapshot struct {
	Version        int       `json:"version"`
	GeneratedAt    time.Time `json:"generatedAt"`
	AppliedThrough int64     `json:"appliedThrough,omitempty"`
	Entries        []Entry   `json:"entries"`
}

// Store keeps the entries in memory and writes them to a JSON file after each
// change. It is safe for concurrent use.
type Store struct {
	path     string
	mu       sync.Mutex
	entries  map[string]Entry
	revision uint64
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
	return s.learn(text, Entry{Device: device, Name: name, Kind: kind}, now)
}

// LearnAction records that text ran one generated catalog action successfully.
func (s *Store) LearnAction(text, action, name string, now time.Time) (Entry, error) {
	return s.learn(text, Entry{Action: action, Name: name}, now)
}

func (s *Store) learn(text string, want Entry, now time.Time) (Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[text]
	if !ok || e.Device != want.Device || e.Kind != want.Kind || e.Action != want.Action {
		e = want
		e.Text = text
		e.Score = 1
		e.Successes = 1
	} else {
		e.Score++
		e.Successes++
	}
	e.Updated = now
	s.entries[text] = e
	s.revision++
	return e, s.saveLocked()
}

// Penalize records that the last run of text was wrong. It undoes the success
// counted at execution time and sets the score to -1, so even a long success
// streak cannot keep a punished action in use.
func (s *Store) Penalize(text, device, kind string, now time.Time) (Entry, bool, error) {
	return s.penalize(text, Entry{Device: device, Kind: kind}, now)
}

// PenalizeAction is Penalize for a generated catalog action.
func (s *Store) PenalizeAction(text, action string, now time.Time) (Entry, bool, error) {
	return s.penalize(text, Entry{Action: action}, now)
}

func (s *Store) penalize(text string, want Entry, now time.Time) (Entry, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.entries[text]
	if !ok || e.Device != want.Device || e.Kind != want.Kind || e.Action != want.Action {
		return Entry{}, false, nil
	}
	e.Failures++
	if e.Successes > 0 {
		e.Successes--
	}
	e.Score = -1
	e.Updated = now
	s.entries[text] = e
	s.revision++
	return e, true, s.saveLocked()
}

// ErrInvalidEntry marks an entry that cannot be stored. Callers can reject it
// without retrying, unlike a write error.
var ErrInvalidEntry = errors.New("invalid learning entry")

// Upsert stores a manually created or edited entry under its normalized text.
// originalText is the key the entry had before the edit; an empty originalText
// creates a new association. When the text changes, the old key is removed.
func (s *Store) Upsert(originalText string, e Entry, now time.Time) (Entry, error) {
	e.Text = Key(e.Text)
	e.Device = strings.TrimSpace(e.Device)
	e.Action = strings.TrimSpace(e.Action)
	e.Kind = strings.TrimSpace(e.Kind)
	e.Name = strings.TrimSpace(e.Name)
	if e.Text == "" {
		return Entry{}, fmt.Errorf("%w: text is empty after normalization", ErrInvalidEntry)
	}
	if (e.Device == "") == (e.Action == "") {
		return Entry{}, fmt.Errorf("%w: entry must name exactly one of device or action", ErrInvalidEntry)
	}
	if e.Device != "" {
		if e.Kind != "turn-on" && e.Kind != "turn-off" {
			return Entry{}, fmt.Errorf("%w: device kind must be turn-on or turn-off", ErrInvalidEntry)
		}
	} else if e.Kind != "" {
		return Entry{}, fmt.Errorf("%w: kind is only valid for a device", ErrInvalidEntry)
	}
	if e.Successes < 0 || e.Failures < 0 {
		return Entry{}, fmt.Errorf("%w: successes and failures must be zero or greater", ErrInvalidEntry)
	}
	e.Updated = now

	s.mu.Lock()
	defer s.mu.Unlock()
	if old := Key(originalText); old != "" && old != e.Text {
		delete(s.entries, old)
	}
	s.entries[e.Text] = e
	s.revision++
	return e, s.saveLocked()
}

// Delete removes the entries for texts and returns how many existed.
func (s *Store) Delete(texts []string) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	removed := 0
	attempted := false
	for _, text := range texts {
		key := Key(text)
		if key == "" {
			continue
		}
		attempted = true
		if _, ok := s.entries[key]; ok {
			delete(s.entries, key)
			removed++
		}
	}
	if !attempted {
		return 0, nil
	}
	if removed > 0 {
		s.revision++
	}
	// Save even when no entry matched: a previous attempt may have changed the
	// in-memory store and failed to write the file, and this retry must repair it.
	return removed, s.saveLocked()
}

// Snapshot returns a copy of every entry at now. The entries are sorted by
// text so repeated snapshots of the same store are stable.
func (s *Store) Snapshot(now time.Time) Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	return Snapshot{
		Version:     SnapshotVersion,
		GeneratedAt: now,
		Entries:     s.sortedEntriesLocked(),
	}
}

// Revision increases after every change to the entries. A publisher compares
// it to avoid sending an identical snapshot at every tick.
func (s *Store) Revision() uint64 {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.revision
}

func (s *Store) sortedEntriesLocked() []Entry {
	entries := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Text < entries[j].Text })
	return entries
}

func (s *Store) saveLocked() error {
	entries := s.sortedEntriesLocked()
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

// Publish posts one snapshot to url. The caller owns the HTTP client and its
// timeout, so an unreachable dashboard cannot delay the voice pipeline.
func Publish(ctx context.Context, client *http.Client, url string, snapshot Snapshot) error {
	if client == nil {
		client = http.DefaultClient
	}
	body, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("encode learning snapshot: %w", err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create learning snapshot request: %w", err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("publish learning snapshot: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		detail := strings.TrimSpace(string(message))
		if detail == "" {
			return fmt.Errorf("publish learning snapshot: %s", response.Status)
		}
		return fmt.Errorf("publish learning snapshot: %s: %s", response.Status, detail)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(response.Body, 1<<20))
	return nil
}
