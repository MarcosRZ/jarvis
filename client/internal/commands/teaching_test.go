package commands

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"jarvis/actions"
	"jarvis/client/internal/data"
	"jarvis/client/internal/learning"
)

type fakeDataProvider struct {
	snapshot data.Snapshot
	err      error
}

func (f *fakeDataProvider) DataSnapshot(context.Context) (data.Snapshot, error) {
	return f.snapshot, f.err
}

func weatherSnapshot(now time.Time, temperature float64) data.Snapshot {
	return data.Snapshot{
		At:      now,
		Weather: []string{"weather.aemet"},
		Entities: map[string]data.Entity{
			"weather.aemet": {
				ID:     "weather.aemet",
				Domain: "weather",
				Name:   "AEMET",
				State:  "rainy",
				Attributes: map[string]any{
					"temperature": temperature,
					"humidity":    float64(70),
				},
				Daily: []data.Forecast{
					{Time: now, Condition: "rainy", Temperature: floatPointer(temperature)},
					{Time: now.AddDate(0, 0, 1), Condition: "sunny", Temperature: floatPointer(14)},
				},
			},
		},
	}
}

func floatPointer(value float64) *float64 {
	return &value
}

func teachingHandler(t *testing.T, now time.Time, devices []Device, controller DeviceController) (*Handler, *learning.Store) {
	t.Helper()
	h, err := New(func() time.Time { return now }, devices, controller)
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	store, err := learning.Open(filepath.Join(t.TempDir(), "learning.json"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	h.UseLearning(store)
	return h, store
}

func TestTeachingFlowStoresACatalogAction(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	devices := &fakeDevices{}
	h, store := teachingHandler(t, now, []Device{lounge}, devices)
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{loungeOff}, executor)
	ctx := context.Background()

	if got := h.Handle(ctx, "Alexa, aprende"); got != ReplyTeachSentence {
		t.Fatalf("aprende without a failure = %q", got)
	}
	if !h.FlowActive() {
		t.Fatal("the teaching flow must be open")
	}
	if got := h.Handle(ctx, "apaga la luz del salón"); got != "¿Es correcta la orden apaga la luz del salón?" {
		t.Fatalf("confirmation = %q", got)
	}
	if got := h.Handle(ctx, "sí"); got != ReplyAskForOrder {
		t.Fatalf("after sí = %q", got)
	}
	if got := h.Handle(ctx, "apaga la luz del salón"); got != ReplyConfirmed {
		t.Fatalf("after the target = %q", got)
	}
	if h.FlowActive() {
		t.Error("a successful ending must close the flow")
	}
	entry, ok := store.Lookup(learningKey("apaga la luz del salón"))
	if !ok || entry.Action != loungeOff.ID || entry.Score != 1 {
		t.Fatalf("entry %+v, %v", entry, ok)
	}
	if got := h.Handle(ctx, "apaga la luz del salón"); got != ReplyDone {
		t.Fatalf("taught action = %q", got)
	}
	if len(executor.calls) != 1 {
		t.Errorf("executor calls %v", executor.calls)
	}
}

func TestTeachingFlowStoresADataTemplate(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, store := teachingHandler(t, now, nil, nil)
	provider := &fakeDataProvider{snapshot: weatherSnapshot(now, 9)}
	h.UseData(provider, nil)
	ctx := context.Background()

	h.Handle(ctx, "aprende")
	if got := h.Handle(ctx, "¿qué temperatura hay fuera?"); got != "¿Es correcta la pregunta qué temperatura hay fuera?" {
		t.Fatalf("confirmation = %q", got)
	}
	if got := h.Handle(ctx, "sí"); got != ReplyAskForAnswer {
		t.Fatalf("after sí = %q", got)
	}
	if got := h.Handle(ctx, "Hay 9 grados."); got != ReplyConfirmed {
		t.Fatalf("after the answer = %q", got)
	}
	entry, ok := store.Lookup(learningKey("¿qué temperatura hay fuera?"))
	if !ok || entry.Template == "" {
		t.Fatalf("entry %+v, %v", entry, ok)
	}
	provider.snapshot = weatherSnapshot(now, 21)
	if got := h.Handle(ctx, "¿qué temperatura hay fuera?"); got != "Hay 21 grados." {
		t.Fatalf("taught template = %q", got)
	}
}

func TestTeachingFlowKeepsAPendingOrder(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, store := teachingHandler(t, now, nil, nil)
	interp := &fakeInterpreter{}
	h.UseInterpreter(interp, InterpretFallback)
	ctx := context.Background()

	h.Handle(ctx, "aprende")
	h.Handle(ctx, "haz magia")
	h.Handle(ctx, "sí")
	if got := h.Handle(ctx, "no sé"); got != ReplyPending {
		t.Fatalf("pending = %q", got)
	}
	entry, ok := store.Lookup(learningKey("haz magia"))
	if !ok || !entry.Pending {
		t.Fatalf("entry %+v, %v", entry, ok)
	}
	if got := h.Handle(ctx, "haz magia"); got != ReplyPending {
		t.Fatalf("a pending entry = %q", got)
	}
}

func TestTeachingFlowAsksAgainAndCancels(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	ctx := context.Background()

	h.Handle(ctx, "aprende")
	if got := h.Handle(ctx, "haz magia"); got != "¿Es correcta la orden haz magia?" {
		t.Fatalf("confirmation = %q", got)
	}
	if got := h.Handle(ctx, "quizá"); got != "¿Es correcta la orden haz magia?" {
		t.Fatalf("an invalid answer repeats the prompt, got %q", got)
	}
	if got := h.Handle(ctx, "no"); got != ReplyTellOrder {
		t.Fatalf("no = %q", got)
	}
	if got := h.Handle(ctx, "cancelar"); got != ReplyOkay {
		t.Fatalf("cancelar = %q", got)
	}
	if h.FlowActive() {
		t.Error("cancelar must close the flow")
	}
}

func TestTeachingReusesARecentFailure(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	ctx := context.Background()

	if got := h.Handle(ctx, "haz magia"); got != "Aún no sé procesar la orden haz magia." {
		t.Fatalf("failure = %q", got)
	}
	if got := h.Handle(ctx, "aprende"); got != "¿Es correcta la orden haz magia?" {
		t.Fatalf("aprende must reuse the failure, got %q", got)
	}
	if got := h.Handle(ctx, "no"); got != ReplyTellOrder {
		t.Fatalf("no = %q", got)
	}
	if got := h.Handle(ctx, "otra cosa"); got != "¿Es correcta la orden otra cosa?" {
		t.Fatalf("new sentence = %q", got)
	}
	if got := h.Handle(ctx, "cancelar"); got != ReplyOkay {
		t.Fatalf("cancelar = %q", got)
	}

	// A failure older than the window is not reused.
	h.Handle(ctx, "haz magia")
	h.now = func() time.Time { return now.Add(2 * failedWindow) }
	if got := h.Handle(ctx, "aprende"); got != ReplyTeachSentence {
		t.Errorf("aprende after the window = %q", got)
	}
}

func TestDataQuestionIsAnsweredBeforeTheModel(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	provider := &fakeDataProvider{snapshot: weatherSnapshot(now, 9)}
	h.UseData(provider, nil)
	interp := &fakeInterpreter{reply: "una respuesta del modelo"}
	h.UseInterpreter(interp, InterpretFallback)

	if got := h.Handle(context.Background(), "¿qué temperatura hace?"); got != "Hay 9 grados y está lloviendo." {
		t.Fatalf("reply %q", got)
	}
	if len(interp.answers) != 0 || len(interp.texts) != 0 {
		t.Errorf("the model was asked: %v %v", interp.texts, interp.answers)
	}
}

func TestOrderWithActionVerbWinsOverTheQuestionShape(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	provider := &fakeDataProvider{snapshot: weatherSnapshot(now, 9)}
	h.UseData(provider, nil)
	executor := &fakeActionExecutor{}
	h.UseActions([]actions.Action{tvOff}, executor)

	// A question-shaped order is not answered with the datum.
	if got := h.Handle(context.Background(), "¿puedes apagar la tele?"); got != ReplyDone {
		t.Fatalf("reply %q, want the action", got)
	}
	if len(executor.calls) != 1 || executor.calls[0].ID != tvOff.ID {
		t.Errorf("executor calls %v", executor.calls)
	}
}

func TestFollowUpReusesTheLastDataTopic(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	provider := &fakeDataProvider{snapshot: weatherSnapshot(now, 9)}
	h.UseData(provider, nil)
	ctx := context.Background()

	if got := h.Handle(ctx, "¿qué tiempo hace?"); got != "Está lloviendo y hay 9 grados." {
		t.Fatalf("weather = %q", got)
	}
	if got := h.Handle(ctx, "¿y mañana?"); got != "Está soleado y hay 14 grados." {
		t.Fatalf("follow-up = %q", got)
	}
	h.now = func() time.Time { return now.Add(2 * dataFollowUp) }
	if got := h.Handle(ctx, "¿y mañana?"); got == "Está soleado y hay 14 grados." {
		t.Errorf("an expired follow-up must not reuse the topic: %q", got)
	}
}

func TestPhysicalStateWithoutADeviceGivesTheDatum(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	h, _ := teachingHandler(t, now, nil, nil)
	provider := &fakeDataProvider{snapshot: weatherSnapshot(now, 9)}
	h.UseData(provider, nil)

	want := "Hay 9 grados y está lloviendo. No tengo ningún aparato de calefacción configurado."
	if got := h.Handle(context.Background(), "tengo frío"); got != want {
		t.Errorf("reply %q, want %q", got, want)
	}
	if got := h.Handle(context.Background(), "¿qué temperatura hace?"); got != "Hay 9 grados y está lloviendo." {
		t.Errorf("a datum question is not a physical state: %q", got)
	}
}

func TestFallbackRefusalStaysTeachable(t *testing.T) {
	now := time.Date(2026, 11, 24, 12, 0, 0, 0, time.Local)
	for _, refusal := range []string{"No, no puedo ayudarte con eso.", "Lo siento, no lo sé."} {
		h, _ := teachingHandler(t, now, nil, nil)
		interp := &fakeInterpreter{answer: refusal}
		h.UseInterpreter(interp, InterpretFallback)

		if got := h.Handle(context.Background(), "haz magia"); got != refusal {
			t.Fatalf("refusal = %q", got)
		}
		if got := h.Handle(context.Background(), "aprende"); got != "¿Es correcta la orden haz magia?" {
			t.Errorf("aprende after %q = %q", refusal, got)
		}
	}
}
