package commands

import (
	"path/filepath"
	"testing"

	"jarvis/actions"
	"jarvis/client/internal/learning"
)

func TestLearnedActionAllowsVariant(t *testing.T) {
	action := actions.Action{
		ID:       "switch.enchufe_socket_1.toggle",
		Name:     "Alternar luz del salon",
		EntityID: "switch.enchufe_socket_1",
		Domain:   "switch",
		Service:  "switch.toggle",
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.LearnAction("macro 1", action.ID, action.Name, at(22, 0)()); err != nil {
		t.Fatal(err)
	}
	h, err := New(at(22, 0), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.UseLearning(store)

	if !h.learnedActionAllows("macro uno", action) {
		t.Error("macro uno should share the word macro with the learned macro 1")
	}
	if !h.learnedActionAllows("Macro 1", action) {
		t.Error("the learned phrase itself should be accepted")
	}
	if h.learnedActionAllows("enciende la tele", action) {
		t.Error("an unrelated sentence must not reuse the learned action")
	}
}

func TestLearnedActionIgnoresPenalizedEntries(t *testing.T) {
	action := actions.Action{ID: "switch.enchufe_socket_1.toggle", Domain: "switch"}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatal(err)
	}
	entry, err := store.LearnAction("macro 1", action.ID, "Macro", at(22, 0)())
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := store.PenalizeAction(entry.Text, action.ID, at(22, 1)()); err != nil {
		t.Fatal(err)
	}
	h, err := New(at(22, 1), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.UseLearning(store)
	if h.learnedActionAllows("macro uno", action) {
		t.Error("a penalized association must not authorize the action")
	}
}
