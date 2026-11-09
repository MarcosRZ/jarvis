package learning

import (
	"os"
	"path/filepath"
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
	if got, ok := reopened.Lookup("no veo nada"); !ok || got != e {
		t.Fatalf("reopened %+v, %v; want %+v", got, ok, e)
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
