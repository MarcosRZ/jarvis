package pipeline

import (
	"context"
	"io"
	"log/slog"
	"slices"
	"sync"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/stt"
	"jarvis/client/internal/tts"
	"jarvis/client/internal/wakeword"
)

// TestDialogueRepeatsThePromptAndAnswersWithoutWakeWord checks the 10 s
// dialogue window: the first turn needs the wake word, the next turns do not,
// and no speech repeats the last prompt.
func TestDialogueRepeatsThePromptAndAnswersWithoutWakeWord(t *testing.T) {
	var mu sync.Mutex
	var said, handled []string
	flow := true
	wakeCalls := 0
	transcribeCalls := 0
	dialogueCalls := 0
	finished := make(chan struct{})

	steps := Steps{
		WaitForWakeWord: func(ctx context.Context) (wakeword.Detection, error) {
			mu.Lock()
			wakeCalls++
			first := wakeCalls == 1
			mu.Unlock()
			if first {
				return wakeword.Detection{Model: "alexa", At: time.Now()}, nil
			}
			close(finished)
			<-ctx.Done()
			return wakeword.Detection{}, ctx.Err()
		},
		Record: func(context.Context) (audio.Recording, error) {
			return audio.Recording{PCM: make([]byte, 3200), Format: audio.SpeechFormat, End: audio.EndSilence}, nil
		},
		RecordDialogue: func(context.Context) (audio.Recording, error) {
			mu.Lock()
			dialogueCalls++
			call := dialogueCalls
			mu.Unlock()
			if call == 1 {
				return audio.Recording{Format: audio.SpeechFormat, End: audio.EndNoSpeech}, nil
			}
			return audio.Recording{PCM: make([]byte, 3200), Format: audio.SpeechFormat, End: audio.EndSilence}, nil
		},
		Transcribe: func(context.Context, audio.Format, []byte) (stt.Result, error) {
			mu.Lock()
			transcribeCalls++
			call := transcribeCalls
			mu.Unlock()
			if call == 1 {
				return stt.Result{Text: "aprende"}, nil
			}
			return stt.Result{Text: "sí"}, nil
		},
		Handle: func(_ context.Context, text string) string {
			mu.Lock()
			handled = append(handled, text)
			if text == "sí" {
				flow = false
			}
			mu.Unlock()
			if text == "aprende" {
				return "¿Es correcta la orden haz magia?"
			}
			return "Entendido."
		},
		Say: func(_ context.Context, text string) (tts.Timing, error) {
			mu.Lock()
			said = append(said, text)
			mu.Unlock()
			return tts.Timing{}, nil
		},
		Continue: func() bool {
			mu.Lock()
			defer mu.Unlock()
			return flow
		},
		DiscardInput: func() {},
	}

	p := New(steps, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()

	select {
	case <-finished:
		cancel()
		<-done
	case <-time.After(3 * time.Second):
		t.Fatal("the dialogue did not close")
	}

	mu.Lock()
	defer mu.Unlock()
	if !slices.Equal(handled, []string{"aprende", "sí"}) {
		t.Errorf("handled %v", handled)
	}
	if !slices.Equal(said, []string{"¿Es correcta la orden haz magia?", "¿Es correcta la orden haz magia?", "Entendido."}) {
		t.Errorf("said %v", said)
	}
	if dialogueCalls != 2 {
		t.Errorf("dialogue recordings %d, want 2", dialogueCalls)
	}
	if wakeCalls != 2 {
		t.Errorf("wake calls %d, want the first turn and the return to listening", wakeCalls)
	}
}
