package commands

import (
	"context"
	"path/filepath"
	"slices"
	"testing"

	"jarvis/client/internal/actions"
	"jarvis/client/internal/learning"
)

var (
	sceneCine = actions.Action{
		ID: "scene.cine.turn_on", Name: "Activar Cine",
		Description: "Activar Cine (scene.cine; tambien: escena)",
		EntityID:    "scene.cine", Domain: "scene", Service: "scene.turn_on",
		Aliases: []string{"escena", "ambiente"},
	}
	catalogLounge = actions.Action{
		ID: "light.lounge.turn_on", Name: "Encender la luz del salón",
		Description: "Encender la luz del salón (light.lounge; tambien: luz, lampara)",
		EntityID:    "light.lounge", Domain: "light", Service: "light.turn_on",
		Aliases: []string{"luz", "lampara"},
	}
	loungeOff = actions.Action{
		ID: "light.lounge.turn_off", Name: "Apagar la luz del salón",
		EntityID: "light.lounge", Domain: "light", Service: "light.turn_off",
		Aliases: []string{"luz", "lampara"},
	}
	loungeToggle = actions.Action{
		ID: "light.lounge.toggle", Name: "Alternar la luz del salón",
		EntityID: "light.lounge", Domain: "light", Service: "light.toggle",
		Aliases: []string{"luz", "lampara"},
	}
	sceneHeat = actions.Action{
		ID: "scene.calentar_1h.turn_on", Name: "Activar Calentar 1h",
		EntityID: "scene.calentar_1h", Domain: "scene", Service: "scene.turn_on",
		Aliases: []string{"escena"},
	}
	tvOn = actions.Action{
		ID: "media_player.lg_tv.turn_on", Name: "Encender LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.turn_on",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvOff = actions.Action{
		ID: "media_player.lg_tv.turn_off", Name: "Apagar LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.turn_off",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvPlay = actions.Action{
		ID: "media_player.lg_tv.play", Name: "Reproducir en LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.media_play",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvPause = actions.Action{
		ID: "media_player.lg_tv.pause", Name: "Pausar LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.media_pause",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvStop = actions.Action{
		ID: "media_player.lg_tv.stop", Name: "Detener LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.media_stop",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvVolumeUp = actions.Action{
		ID: "media_player.lg_tv.volume_up", Name: "Subir el volumen de LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.volume_up",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvVolumeDown = actions.Action{
		ID: "media_player.lg_tv.volume_down", Name: "Bajar el volumen de LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.volume_down",
		Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvMute = actions.Action{
		ID: "media_player.lg_tv.mute", Name: "Silenciar LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.volume_mute",
		Data: map[string]any{"is_volume_muted": true}, Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
	tvUnmute = actions.Action{
		ID: "media_player.lg_tv.unmute", Name: "Quitar el silencio a LG TV",
		EntityID: "media_player.lg_tv", Domain: "media_player", Service: "media_player.volume_mute",
		Data: map[string]any{"is_volume_muted": false}, Aliases: []string{"tele", "television", "tv", "reproductor"},
	}
)

// fakeActionExecutor records each catalog action and can fail.
type fakeActionExecutor struct {
	calls []actions.Action
	err   error
}

func (f *fakeActionExecutor) ExecuteAction(_ context.Context, action actions.Action) error {
	f.calls = append(f.calls, action)
	return f.err
}

// fakeActionInterpreter returns one catalog action ID. It also implements
// InterpretContext through the embedded fake, so it is a full Interpreter.
type fakeActionInterpreter struct {
	fakeInterpreter
	actionTexts []string
	actionLists [][]actions.Action
	actionID    string
	actionIDs   []string
}

func (f *fakeActionInterpreter) InterpretActions(_ context.Context, text string, list []actions.Action, _ string) (Interpretation, error) {
	f.actionTexts = append(f.actionTexts, text)
	f.actionLists = append(f.actionLists, list)
	ids := f.actionIDs
	if len(ids) == 0 && f.actionID != "" {
		ids = []string{f.actionID}
	}
	commands := make([]Command, 0, len(ids))
	for _, id := range ids {
		commands = append(commands, Command{Kind: KindAction, Action: actions.Action{ID: id}})
	}
	return Interpretation{Commands: commands}, f.err
}

func TestHandleRunsCatalogActionAndLearnsIt(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	if problems := h.UseActions([]actions.Action{tvOn}, executor); len(problems) != 0 {
		t.Fatalf("UseActions: %v", problems)
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	interp := &fakeActionInterpreter{actionID: tvOn.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "necesito ver la tele"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != tvOn.ID {
		t.Errorf("executor calls %+v", executor.calls)
	}
	if !slices.Equal(interp.actionTexts, []string{"necesito ver la tele"}) {
		t.Errorf("interpreter texts %v", interp.actionTexts)
	}
	entry, ok := store.Lookup(learningKey("necesito ver la tele"))
	if !ok || entry.Action != tvOn.ID {
		t.Errorf("learned entry %+v, %v; want action %s", entry, ok, tvOn.ID)
	}

	// The learned sentence runs without the model.
	h.UseInterpreter(nil, InterpretFallback)
	if got := h.Handle(context.Background(), "necesito ver la tele"); got != ReplyDone {
		t.Errorf("learned reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 2 {
		t.Errorf("executor calls %+v, want the learned replay", executor.calls)
	}
}

func TestHandleGuardRejectsAnUnrelatedCatalogAction(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{sceneCine}, executor)
	interp := &fakeActionInterpreter{actionID: sceneCine.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "cuéntame un chiste"); got == ReplyDone {
		t.Errorf("reply %q, want a rejection", got)
	}
	if len(executor.calls) != 0 {
		t.Errorf("executor calls %+v for an unrelated sentence", executor.calls)
	}
	if len(interp.actionTexts) != 1 {
		t.Errorf("interpreter texts %v", interp.actionTexts)
	}
}

func TestHandleAcceptsACatalogActionForAPhysicalState(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{catalogLounge}, executor)
	interp := &fakeActionInterpreter{actionID: catalogLounge.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "no veo nada"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != catalogLounge.ID {
		t.Errorf("executor calls %+v", executor.calls)
	}
}

func TestHandlePenalizesACatalogAction(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{sceneCine}, executor)
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	interp := &fakeActionInterpreter{actionID: sceneCine.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "pon la escena cine"); got != ReplyDone {
		t.Fatalf("reply %q", got)
	}
	if got := h.Handle(context.Background(), "mal"); got != ReplyPenalized {
		t.Fatalf("penalty reply %q", got)
	}
	if got := h.Handle(context.Background(), "pon la escena cine"); got == ReplyDone {
		t.Errorf("reply %q after a penalty", got)
	}
	if len(executor.calls) != 1 {
		t.Errorf("executor calls %+v after a penalty", executor.calls)
	}
}

func TestSetActionsRejectsBadAndDuplicateActions(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	bad := sceneCine
	bad.ID = "scene.bad.turn_on"
	bad.EntityID = "scene.bad"
	bad.Service = "scene.explode"
	duplicate := sceneCine
	duplicate.Name = "Otra escena"
	problems := h.UseActions([]actions.Action{sceneCine, bad, duplicate}, &fakeActionExecutor{})
	if len(problems) != 2 {
		t.Fatalf("problems %v, want 2", problems)
	}
	if got := h.Actions(); len(got) != 1 || got[0].ID != sceneCine.ID {
		t.Errorf("actions %+v", got)
	}
}

func TestHandleDoesNotRunACatalogActionWithoutAnExecutor(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	h.SetActions([]actions.Action{sceneCine})
	interp := &fakeActionInterpreter{actionID: sceneCine.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "pon la escena cine"); got != ReplyUnknown {
		t.Errorf("reply %q, want %q", got, ReplyUnknown)
	}
	if len(interp.actionTexts) != 0 {
		t.Errorf("interpreter called without an executor: %v", interp.actionTexts)
	}
}

func TestHandleReportsACatalogActionFailure(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{err: context.DeadlineExceeded}
	h.UseActions([]actions.Action{sceneCine}, executor)
	h.UseInterpreter(&fakeActionInterpreter{actionID: sceneCine.ID}, InterpretFallback)

	if got := h.Handle(context.Background(), "pon la escena cine"); got != "No he podido activar Cine." {
		t.Errorf("reply %q", got)
	}
}

func TestMatchCatalogAction(t *testing.T) {
	list := []actions.Action{
		sceneCine, sceneHeat, catalogLounge, loungeOff, loungeToggle,
		tvOn, tvOff, tvPlay, tvPause, tvStop, tvVolumeUp, tvVolumeDown, tvMute, tvUnmute,
	}
	tests := []struct {
		text string
		want string
	}{
		{"enciende la tele", tvOn.ID},
		{"pon la tele", tvOn.ID},
		{"apaga la tele", tvOff.ID},
		{"silencia la tele", tvMute.ID},
		{"quita el silencio de la tele", tvUnmute.ID},
		{"sube el volumen de la tele", tvVolumeUp.ID},
		{"baja el volumen de la tele", tvVolumeDown.ID},
		{"pausa la tele", tvPause.ID},
		{"reproduce la tele", tvPlay.ID},
		{"deten la tele", tvStop.ID},
		{"activa la escena calentar", sceneHeat.ID},
		{"calienta la casa durante una hora", sceneHeat.ID},
		{"activa la escena cine", sceneCine.ID},
		{"enciende la luz del salón", catalogLounge.ID},
		{"pon la luz del salón", catalogLounge.ID},
		{"apaga la luz del salón", loungeOff.ID},
		{"alterna la luz del salón", loungeToggle.ID},
		{"pon la tele en modo cine", ""},
		{"cuéntame un chiste", ""},
		{"enciende el enchufe", ""},
	}
	for _, tt := range tests {
		got, ok := matchCatalogAction(tt.text, list)
		if tt.want == "" {
			if ok {
				t.Errorf("matchCatalogAction(%q) = %s, want no match", tt.text, got.ID)
			}
			continue
		}
		if !ok || got.ID != tt.want {
			t.Errorf("matchCatalogAction(%q) = %s, %v; want %s", tt.text, got.ID, ok, tt.want)
		}
	}

	second := catalogLounge
	second.ID = "light.lounge2.turn_on"
	second.EntityID = "light.lounge2"
	second.Name = "Encender la luz del salón 2"
	if got, ok := matchCatalogAction("enciende la luz del salón", []actions.Action{catalogLounge, second}); ok {
		t.Errorf("ambiguous match returned %s", got.ID)
	}
}

func TestActionNamedIgnoresTheGeneratedVerb(t *testing.T) {
	if !actionNamed("enciende la tele", tvOn) {
		t.Error("the TV alias did not match")
	}
	if actionNamed("enciende la tele", catalogLounge) {
		t.Error("the verb alone matched the light")
	}
	if !actionNamed("activa la escena calentar", sceneHeat) {
		t.Error("the scene name did not match")
	}
	if !actionNamed("calienta la casa", sceneHeat) {
		t.Error("the conjugated scene verb did not match")
	}
}

func TestSimilarWord(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"calienta", "calentar", true},
		{"enciende", "encender", true},
		{"cierra", "cerrar", true},
		{"tele", "television", true},
		{"casa", "calentar", false},
		{"sala", "salir", false},
		{"luz", "salon", false},
	}
	for _, tt := range tests {
		if got := similarWord(tt.a, tt.b); got != tt.want {
			t.Errorf("similarWord(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestHandleUsesTheCatalogMatcherWithoutTheModel(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	if problems := h.UseActions([]actions.Action{tvOn, tvOff}, executor); len(problems) != 0 {
		t.Fatalf("UseActions: %v", problems)
	}
	// A wrong model answer must not win over the explicit verb and entity.
	interp := &fakeActionInterpreter{actionID: tvOff.ID}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "enciende la tele"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != tvOn.ID {
		t.Errorf("executor calls %+v, want %s", executor.calls, tvOn.ID)
	}
	if len(interp.actionTexts) != 0 {
		t.Errorf("the model was called for a deterministic match: %v", interp.actionTexts)
	}
	if got := h.Handle(context.Background(), "apaga la tele"); got != ReplyDone {
		t.Errorf("reply %q", got)
	}
	if len(executor.calls) != 2 || executor.calls[1].ID != tvOff.ID {
		t.Errorf("executor calls %+v, want %s", executor.calls, tvOff.ID)
	}
}

func TestMatchCatalogActions(t *testing.T) {
	list := []actions.Action{catalogLounge, loungeOff, tvOn, tvOff, sceneCine}
	tests := []struct {
		text string
		want []string
	}{
		{"enciende la luz del salón", []string{catalogLounge.ID}},
		{"enciende la luz del salón y apaga la tele", []string{catalogLounge.ID, tvOff.ID}},
		{"quiero que enciendas la luz del salón y apagues la tele", []string{catalogLounge.ID, tvOff.ID}},
		{"enciende la tele y apaga la tele", []string{tvOn.ID, tvOff.ID}},
		{"enciende la luz del salón y apaga la luz del salón", []string{catalogLounge.ID, loungeOff.ID}},
		{"enciende la luz del salón y pon la tele en modo cine", nil},
		{"enciende la luz del salón y la tele", nil},
	}
	for _, tt := range tests {
		got, ok := matchCatalogActions(tt.text, list)
		ids := make([]string, len(got))
		for i, action := range got {
			ids[i] = action.ID
		}
		if tt.want == nil {
			if ok {
				t.Errorf("matchCatalogActions(%q) = %v, want no match", tt.text, ids)
			}
			continue
		}
		if !ok || !slices.Equal(ids, tt.want) {
			t.Errorf("matchCatalogActions(%q) = %v, %v; want %v", tt.text, ids, ok, tt.want)
		}
	}
}

func TestHandleRunsSeveralCatalogActionsInOrder(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	if problems := h.UseActions([]actions.Action{catalogLounge, loungeOff, tvOn, tvOff}, executor); len(problems) != 0 {
		t.Fatalf("UseActions: %v", problems)
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	// The model would choose only one; the deterministic matcher must win.
	interp := &fakeActionInterpreter{actionIDs: []string{tvOn.ID}}
	h.UseInterpreter(interp, InterpretFallback)

	text := "quiero que enciendas la luz del salón y apagues la tele"
	if got := h.Handle(context.Background(), text); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 2 || executor.calls[0].ID != catalogLounge.ID || executor.calls[1].ID != tvOff.ID {
		t.Errorf("executor calls %+v, want %s then %s", executor.calls, catalogLounge.ID, tvOff.ID)
	}
	if len(interp.actionTexts) != 0 {
		t.Errorf("the model was called for a deterministic match: %v", interp.actionTexts)
	}
	if _, ok := store.Lookup(learningKey(text)); ok {
		t.Error("a multi-action sentence was stored as one association")
	}
	if got := h.Handle(context.Background(), "mal"); got != ReplyNothingToCorrect {
		t.Errorf("reply %q after a multi-action command, want %q", got, ReplyNothingToCorrect)
	}
}

func TestHandleRunsSeveralModelActions(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	if problems := h.UseActions([]actions.Action{catalogLounge, loungeOff, tvOn, tvOff}, executor); len(problems) != 0 {
		t.Fatalf("UseActions: %v", problems)
	}
	interp := &fakeActionInterpreter{actionIDs: []string{tvOn.ID, loungeOff.ID}}
	h.UseInterpreter(interp, InterpretFallback)

	text := "quiero ver la tele y apagar la luz del salón"
	if got := h.Handle(context.Background(), text); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(executor.calls) != 2 || executor.calls[0].ID != tvOn.ID || executor.calls[1].ID != loungeOff.ID {
		t.Errorf("executor calls %+v, want %s then %s", executor.calls, tvOn.ID, loungeOff.ID)
	}
	if len(interp.actionTexts) != 1 {
		t.Errorf("interpreter texts %v", interp.actionTexts)
	}
}

func TestHandleMatchesTheCatalogBeforeHomeAssistant(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{tvOn, tvOff}, executor)
	conversation := &fakeConversation{handled: true, reply: "Home Assistant"}
	h.UseConversation(conversation)
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)

	if got := h.Handle(context.Background(), "enciende la tele"); got != ReplyDone {
		t.Errorf("reply %q, want %q", got, ReplyDone)
	}
	if len(conversation.texts) != 0 {
		t.Errorf("Home Assistant was asked first: %v", conversation.texts)
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != tvOn.ID {
		t.Errorf("executor calls %+v", executor.calls)
	}
	entry, ok := store.Lookup(learningKey("enciende la tele"))
	if !ok || entry.Action != tvOn.ID {
		t.Errorf("learned entry %+v, %v", entry, ok)
	}
}

func TestGuardChecksTheVerbForSensitiveDomains(t *testing.T) {
	h, err := New(at(22, 15), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	unlock := actions.Action{
		ID: "lock.door.unlock", Name: "Desbloquear la puerta",
		EntityID: "lock.door", Domain: "lock", Service: "lock.unlock",
		Aliases: []string{"puerta"},
	}
	lock := actions.Action{
		ID: "lock.door.lock", Name: "Bloquear la puerta",
		EntityID: "lock.door", Domain: "lock", Service: "lock.lock",
		Aliases: []string{"puerta"},
	}
	tests := []struct {
		text   string
		action actions.Action
		want   bool
	}{
		{"la puerta", unlock, false},
		{"abre la puerta", unlock, true},
		{"cierra la puerta", lock, true},
		{"cierra la puerta", unlock, false},
		{"abre la puerta", lock, false},
		{"la escena cine", sceneCine, false},
		{"activa la escena cine", sceneCine, true},
	}
	for _, tt := range tests {
		if got := h.acceptsAction(tt.text, tt.action); got != tt.want {
			t.Errorf("acceptsAction(%q, %s) = %v, want %v", tt.text, tt.action.ID, got, tt.want)
		}
	}
}

func TestMatchCatalogActionDefersParameterizedSentences(t *testing.T) {
	climateOn := actions.Action{
		ID: "climate.aire.turn_on", Name: "Encender El aire",
		EntityID: "climate.aire", Domain: "climate", Service: "climate.turn_on",
		Aliases: []string{"aire", "climatizador"},
	}
	heat := actions.Action{
		ID: "climate.aire.mode_heat", Name: "Poner El aire en modo Calor",
		EntityID: "climate.aire", Domain: "climate", Service: "climate.set_hvac_mode",
		Data: map[string]any{"hvac_mode": "heat"}, Aliases: []string{"aire", "calefaccion"},
	}
	list := []actions.Action{climateOn, heat}
	if got, ok := matchCatalogAction("enciende el aire", list); !ok || got.ID != climateOn.ID {
		t.Errorf("enciende el aire = %s, %v", got.ID, ok)
	}
	for _, text := range []string{
		"pon el aire a 22 grados",
		"pon el aire en calor",
		"pon el aire en modo calor",
		"sube la temperatura del aire",
	} {
		if got, ok := matchCatalogAction(text, list); ok {
			t.Errorf("%q matched %s; parameters must go to Home Assistant or the model", text, got.ID)
		}
	}
}
