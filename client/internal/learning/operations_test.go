package learning

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestNormalizeAndKey(t *testing.T) {
	cases := []struct {
		in   string
		norm string
		key  string
	}{
		{"Enciende la luz, por favor!", "enciende la luz por favor", "enciende la luz"},
		{"  ¿Qué   hora es?  ", "que hora es", "que hora es"},
		{"Apaga la televisión", "apaga la television", "apaga la television"},
		{"!!!", "", ""},
	}
	for _, c := range cases {
		if got := Normalize(c.in); got != c.norm {
			t.Errorf("Normalize(%q) = %q, want %q", c.in, got, c.norm)
		}
		if got := Key(c.in); got != c.key {
			t.Errorf("Key(%q) = %q, want %q", c.in, got, c.key)
		}
	}
}

func TestStoreUpsertAndDelete(t *testing.T) {
	path := filepath.Join(t.TempDir(), "learning.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)
	entry, err := store.Upsert("", Entry{
		Text:      "Enciende la luz, por favor!",
		Device:    " light.lounge ",
		Name:      " la luz del salón ",
		Kind:      "turn-on",
		Score:     2,
		Successes: 3,
	}, now)
	if err != nil {
		t.Fatalf("Upsert new: %v", err)
	}
	if entry.Text != "enciende la luz" || entry.Device != "light.lounge" || entry.Name != "la luz del salón" {
		t.Fatalf("entry %+v", entry)
	}
	if !entry.Updated.Equal(now) {
		t.Errorf("updated %v, want %v", entry.Updated, now)
	}
	if store.Len() != 1 {
		t.Fatalf("Len %d, want 1", store.Len())
	}

	edited, err := store.Upsert("enciende la luz", Entry{
		Text:   "Apaga la luz",
		Device: "light.lounge",
		Kind:   "turn-off",
		Score:  -1,
	}, now.Add(time.Minute))
	if err != nil {
		t.Fatalf("Upsert edit: %v", err)
	}
	if edited.Text != "apaga la luz" || edited.Score != -1 || edited.Successes != 0 {
		t.Fatalf("edited entry %+v", edited)
	}
	if _, ok := store.Lookup("enciende la luz"); ok {
		t.Error("old key still exists after editing the text")
	}
	if _, ok := store.Lookup("apaga la luz"); !ok {
		t.Error("new key does not exist after editing the text")
	}

	reopened, err := Open(path)
	if err != nil {
		t.Fatalf("Open again: %v", err)
	}
	if got, ok := reopened.Lookup("apaga la luz"); !ok || got.Kind != "turn-off" || got.Score != -1 {
		t.Fatalf("reopened %+v, %v", got, ok)
	}

	removed, err := store.Delete([]string{"APAGA LA LUZ", "not there"})
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if removed != 1 || store.Len() != 0 {
		t.Fatalf("Delete removed %d, Len %d; want 1, 0", removed, store.Len())
	}
}

func TestStoreUpsertRejectsBadEntries(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now()
	cases := []Entry{
		{Text: "!!!", Device: "light.a", Kind: "turn-on"},
		{Text: "sin objetivo"},
		{Text: "dos objetivos", Device: "light.a", Action: "scene.a.turn_on", Kind: "turn-on"},
		{Text: "sin tipo", Device: "light.a"},
		{Text: "tipo incorrecto", Device: "light.a", Kind: "toggle"},
		{Text: "accion con tipo", Action: "scene.a.turn_on", Kind: "turn-on"},
		{Text: "negativo", Device: "light.a", Kind: "turn-on", Successes: -1},
	}
	for _, entry := range cases {
		if _, err := store.Upsert("", entry, now); !errors.Is(err, ErrInvalidEntry) {
			t.Errorf("Upsert(%+v) error %v, want ErrInvalidEntry", entry, err)
		}
	}
	if store.Len() != 0 {
		t.Fatalf("Len %d, want 0", store.Len())
	}
}

func TestStoreApplyOperation(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	now := time.Now()
	entry := Entry{
		Text:   "pon la escena cine",
		Action: "scene.cine.turn_on",
		Name:   "Activar Cine",
		Score:  1,
	}
	applied, err := store.ApplyOperation(Operation{Kind: OperationUpsert, Entry: &entry}, now)
	if err != nil || !applied {
		t.Fatalf("ApplyOperation upsert: applied %v, err %v", applied, err)
	}
	if _, ok := store.Lookup("pon la escena cine"); !ok {
		t.Fatal("upsert did not store the entry")
	}
	applied, err = store.ApplyOperation(Operation{Kind: OperationDelete, Texts: []string{"pon la escena cine"}}, now)
	if err != nil || !applied {
		t.Fatalf("ApplyOperation delete: applied %v, err %v", applied, err)
	}
	if store.Len() != 0 {
		t.Fatalf("Len %d, want 0", store.Len())
	}
	applied, err = store.ApplyOperation(Operation{Kind: OperationDelete, Texts: []string{"missing"}}, now)
	if err != nil || applied {
		t.Fatalf("ApplyOperation missing delete: applied %v, err %v", applied, err)
	}
	if _, err := store.ApplyOperation(Operation{Kind: "replace", Entry: &entry}, now); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("unknown kind error %v, want ErrInvalidOperation", err)
	}
	if _, err := store.ApplyOperation(Operation{Kind: OperationUpsert}, now); !errors.Is(err, ErrInvalidOperation) {
		t.Fatalf("missing entry error %v, want ErrInvalidOperation", err)
	}
}

func TestOperationsURL(t *testing.T) {
	cases := map[string]string{
		"http://127.0.0.1:3005/api/learning":  "http://127.0.0.1:3005/api/learning/operations",
		"http://127.0.0.1:3005/api/learning/": "http://127.0.0.1:3005/api/learning/operations",
		"http://host/operations":              "http://host/operations",
	}
	for in, want := range cases {
		if got := OperationsURL(in); got != want {
			t.Errorf("OperationsURL(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFetchOperations(t *testing.T) {
	var method, after string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		method = r.Method
		after = r.URL.Query().Get("after")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(operationsResponse{Operations: []Operation{{
			ID:           42,
			Kind:         OperationUpsert,
			OriginalText: "hola",
			Entry:        &Entry{Text: "hola", Device: "light.a", Kind: "turn-on"},
		}}})
	}))
	t.Cleanup(server.Close)

	operations, err := FetchOperations(context.Background(), server.Client(), server.URL+"/operations", 41)
	if err != nil {
		t.Fatalf("FetchOperations: %v", err)
	}
	if method != http.MethodGet || after != "41" {
		t.Errorf("method %q, after %q; want GET, 41", method, after)
	}
	if len(operations) != 1 || operations[0].ID != 42 || operations[0].Entry == nil || operations[0].Entry.Text != "hola" {
		t.Fatalf("operations %+v", operations)
	}
}

func TestFetchOperationsReportsAnErrorStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "queue unavailable", http.StatusServiceUnavailable)
	}))
	t.Cleanup(server.Close)

	_, err := FetchOperations(context.Background(), server.Client(), server.URL, 0)
	if err == nil {
		t.Fatal("FetchOperations returned no error")
	}
	for _, want := range []string{"503", "queue unavailable"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}
