package learning

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreLearnsAndPersists(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning.json")
	now := time.Date(2026, 10, 28, 23, 0, 0, 0, time.Local)
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("Len %d, want 0", store.Len())
	}
	e, err := store.Learn("no veo nada", "light.lounge", "la luz del salón", "turn-on", now)
	if err != nil {
		t.Fatalf("Learn: %v", err)
	}
	if e.Score != 1 || e.Successes != 1 || e.Failures != 0 {
		t.Fatalf("entry %+v", e)
	}
	if _, err := store.Learn("no veo nada", "light.lounge", "la luz del salón", "turn-on", now); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	e, ok := store.Lookup("no veo nada")
	if !ok || e.Score != 2 || e.Successes != 2 {
		t.Fatalf("lookup %+v, %v", e, ok)
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	got, ok := reopened.Lookup("no veo nada")
	if !ok {
		t.Fatalf("reopened lookup failed; want %+v", e)
	}
	// time.Time values from JSON use a different location pointer than the
	// input time, so compare fields and the instant instead of the struct.
	if got.Text != e.Text || got.Device != e.Device || got.Name != e.Name ||
		got.Kind != e.Kind || got.Action != e.Action || got.Score != e.Score ||
		got.Successes != e.Successes || got.Failures != e.Failures ||
		!got.Updated.Equal(e.Updated) {
		t.Fatalf("reopened %+v; want %+v", got, e)
	}
}

func TestStoreReplacesADifferentAction(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now()
	if _, err := store.Learn("hay poca claridad", "light.kitchen", "la luz de la cocina", "turn-on", now); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	e, err := store.Learn("hay poca claridad", "light.lounge", "la luz del salón", "turn-on", now)
	if err != nil {
		t.Fatalf("Learn: %v", err)
	}
	if e.Device != "light.lounge" || e.Score != 1 || e.Successes != 1 {
		t.Fatalf("entry %+v", e)
	}
}

func TestStorePenalize(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now()
	if _, err := store.Learn("no veo nada", "light.lounge", "la luz del salón", "turn-on", now); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	e, ok, err := store.Penalize("no veo nada", "light.lounge", "turn-on", now)
	if err != nil {
		t.Fatalf("Penalize: %v", err)
	}
	if !ok {
		t.Fatal("Penalize found no entry")
	}
	if e.Score != -1 || e.Successes != 0 || e.Failures != 1 {
		t.Fatalf("entry %+v", e)
	}
	if _, ok, err := store.Penalize("no veo nada", "light.kitchen", "turn-on", now); err != nil || ok {
		t.Fatalf("wrong device: ok %v, err %v", ok, err)
	}
}

func TestStoreLearnsCatalogActions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning.json")
	now := time.Now()
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if _, err := store.LearnAction("pon la escena cine", "scene.cine.turn_on", "Activar Cine", now); err != nil {
		t.Fatalf("LearnAction: %v", err)
	}
	e, ok := store.Lookup("pon la escena cine")
	if !ok || e.Action != "scene.cine.turn_on" || e.Score != 1 || e.Successes != 1 {
		t.Fatalf("lookup %+v, %v", e, ok)
	}
	if _, err := store.LearnAction("pon la escena cine", "scene.cine.turn_on", "Activar Cine", now); err != nil {
		t.Fatalf("LearnAction again: %v", err)
	}
	e, ok = store.Lookup("pon la escena cine")
	if !ok || e.Score != 2 || e.Successes != 2 {
		t.Fatalf("second lookup %+v, %v", e, ok)
	}
	if _, ok, err := store.PenalizeAction("pon la escena cine", "scene.other.turn_on", now); err != nil || ok {
		t.Fatalf("wrong action: ok %v, err %v", ok, err)
	}
	e, ok, err = store.PenalizeAction("pon la escena cine", "scene.cine.turn_on", now)
	if err != nil || !ok || e.Score != -1 || e.Failures != 1 {
		t.Fatalf("PenalizeAction: %+v, %v, %v", e, ok, err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	if got, ok := reopened.Lookup("pon la escena cine"); !ok || got.Action != "scene.cine.turn_on" || got.Score != -1 {
		t.Fatalf("reopened %+v, %v", got, ok)
	}
}

func TestStoreRejectsBadFiles(t *testing.T) {
	if _, err := Open(""); err == nil {
		t.Error("Open with empty path succeeded")
	}
	path := filepath.Join(t.TempDir(), "learning.json")
	if err := os.WriteFile(path, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err == nil {
		t.Error("Open with invalid JSON succeeded")
	}
}

func TestStoreAcceptsAnEmptyFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning.json")
	if err := os.WriteFile(path, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if store.Len() != 0 {
		t.Fatalf("Len %d, want 0", store.Len())
	}
}

func TestStoreSnapshotTracksChanges(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)
	if got := store.Revision(); got != 0 {
		t.Fatalf("revision %d, want 0", got)
	}
	if _, err := store.Learn("z apaga la luz", "light.z", "la luz z", "turn-off", now); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	if _, err := store.Learn("a enciende la luz", "light.a", "la luz a", "turn-on", now); err != nil {
		t.Fatalf("Learn: %v", err)
	}
	if got := store.Revision(); got != 2 {
		t.Fatalf("revision %d, want 2", got)
	}

	at := now.Add(time.Minute)
	snapshot := store.Snapshot(at)
	if snapshot.Version != SnapshotVersion {
		t.Errorf("version %d, want %d", snapshot.Version, SnapshotVersion)
	}
	if !snapshot.GeneratedAt.Equal(at) {
		t.Errorf("generatedAt %v, want %v", snapshot.GeneratedAt, at)
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries %d, want 2", len(snapshot.Entries))
	}
	if snapshot.Entries[0].Text != "a enciende la luz" || snapshot.Entries[1].Text != "z apaga la luz" {
		t.Errorf("entries are not sorted by text: %+v", snapshot.Entries)
	}

	if _, ok, err := store.Penalize("missing", "light.a", "turn-on", at); err != nil || ok {
		t.Fatalf("Penalize missing: ok %v, err %v", ok, err)
	}
	if got := store.Revision(); got != 2 {
		t.Errorf("revision after a missed penalize %d, want 2", got)
	}
	if _, ok, err := store.Penalize("a enciende la luz", "light.a", "turn-on", at); err != nil || !ok {
		t.Fatalf("Penalize: ok %v, err %v", ok, err)
	}
	if got := store.Revision(); got != 3 {
		t.Errorf("revision after a penalize %d, want 3", got)
	}
	snapshot = store.Snapshot(at)
	if snapshot.Entries[0].Score != -1 {
		t.Errorf("penalized score %d, want -1", snapshot.Entries[0].Score)
	}
}

func TestPublishPostsTheSnapshot(t *testing.T) {
	var method, contentType string
	var got Snapshot
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		contentType = r.Header.Get("Content-Type")
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Errorf("decode body: %v", err)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	t.Cleanup(server.Close)

	want := Snapshot{
		Version:     SnapshotVersion,
		GeneratedAt: time.Date(2026, 11, 17, 12, 0, 0, 0, time.UTC),
		Entries: []Entry{{
			Text:      "apaga la luz",
			Device:    "switch.luz",
			Name:      "luz del salon",
			Kind:      "turn-off",
			Score:     2,
			Successes: 2,
			Updated:   time.Date(2026, 11, 17, 11, 59, 0, 0, time.UTC),
		}},
	}
	if err := Publish(context.Background(), server.Client(), server.URL, want); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if method != http.MethodPost {
		t.Errorf("method %q, want POST", method)
	}
	if contentType != "application/json" {
		t.Errorf("content type %q, want application/json", contentType)
	}
	if got.Version != want.Version || !got.GeneratedAt.Equal(want.GeneratedAt) || len(got.Entries) != 1 {
		t.Fatalf("published %+v, want %+v", got, want)
	}
	if got.Entries[0].Text != want.Entries[0].Text || got.Entries[0].Device != want.Entries[0].Device || got.Entries[0].Score != want.Entries[0].Score {
		t.Errorf("published entry %+v, want %+v", got.Entries[0], want.Entries[0])
	}
}

func TestPublishReportsAnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "snapshot rejected", http.StatusBadRequest)
	}))
	t.Cleanup(server.Close)

	err := Publish(context.Background(), server.Client(), server.URL, Snapshot{Version: SnapshotVersion})
	if err == nil {
		t.Fatal("Publish returned no error")
	}
	for _, want := range []string{"400", "snapshot rejected"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
