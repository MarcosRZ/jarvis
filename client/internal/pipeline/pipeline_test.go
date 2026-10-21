package pipeline

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"regexp"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/stt"
	"jarvis/client/internal/tts"
	"jarvis/client/internal/wakeword"
)

// request is one scripted interaction after a wake word.
type request struct {
	noSpeech      bool
	text          string
	transcribeErr error
	sayErr        error
}

// fake plays all steps from a script and records each call in calls.
type fake struct {
	mu       sync.Mutex
	script   []request
	current  request
	calls    []string
	said     []string
	handled  []string
	waitErr  error
	recErr   error
	hangSay  bool
	finished chan struct{}
}

func newFake(script ...request) *fake {
	return &fake{script: script, finished: make(chan struct{})}
}

func (f *fake) call(name string) {
	f.mu.Lock()
	f.calls = append(f.calls, name)
	f.mu.Unlock()
}

func (f *fake) steps() Steps {
	return Steps{
		WaitForWakeWord: func(ctx context.Context) (wakeword.Detection, error) {
			f.call("wait")
			if f.waitErr != nil {
				return wakeword.Detection{}, f.waitErr
			}
			f.mu.Lock()
			if len(f.script) == 0 {
				f.mu.Unlock()
				close(f.finished)
				<-ctx.Done()
				return wakeword.Detection{}, ctx.Err()
			}
			f.current, f.script = f.script[0], f.script[1:]
			f.mu.Unlock()
			return wakeword.Detection{Model: "alexa", At: time.Now()}, nil
		},
		Record: func(context.Context) (audio.Recording, error) {
			f.call("record")
			if f.recErr != nil {
				return audio.Recording{}, f.recErr
			}
			if f.current.noSpeech {
				return audio.Recording{Format: audio.SpeechFormat, End: audio.EndNoSpeech}, nil
			}
			return audio.Recording{PCM: make([]byte, 3200), Format: audio.SpeechFormat, End: audio.EndSilence}, nil
		},
		Transcribe: func(context.Context, audio.Format, []byte) (stt.Result, error) {
			f.call("transcribe")
			return stt.Result{Text: f.current.text}, f.current.transcribeErr
		},
		Handle: func(_ context.Context, text string) string {
			f.call("handle")
			f.mu.Lock()
			f.handled = append(f.handled, text)
			f.mu.Unlock()
			return "reply to " + text
		},
		Say: func(ctx context.Context, text string) (tts.Timing, error) {
			f.call("say")
			f.mu.Lock()
			f.said = append(f.said, text)
			f.mu.Unlock()
			if f.hangSay {
				<-ctx.Done()
				return tts.Timing{}, ctx.Err()
			}
			return tts.Timing{}, f.current.sayErr
		},
		DiscardInput: func() { f.call("discard") },
	}
}

// runScript runs the pipeline until the script is done, then stops it.
func runScript(t *testing.T, f *fake) (string, error) {
	t.Helper()
	var logs bytes.Buffer
	p := New(f.steps(), slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	select {
	case <-f.finished:
		cancel()
		err := <-done
		return logs.String(), err
	case err := <-done:
		return logs.String(), err
	case <-time.After(3 * time.Second):
		t.Fatal("pipeline did not finish the script")
		return "", nil
	}
}

var transition = regexp.MustCompile(`to=([A-Z_]+)`)

func states(logs string) []string {
	var out []string
	for _, m := range transition.FindAllStringSubmatch(logs, -1) {
		out = append(out, m[1])
	}
	return out
}

func TestRunAnswersOneCommand(t *testing.T) {
	f := newFake(request{text: "¿Qué hora es?"})
	logs, err := runScript(t, f)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error %v, want context.Canceled", err)
	}
	if !slices.Equal(f.handled, []string{"¿Qué hora es?"}) || !slices.Equal(f.said, []string{"reply to ¿Qué hora es?"}) {
		t.Errorf("handled %q, said %q", f.handled, f.said)
	}
	want := []string{"LISTENING", "RECORDING", "TRANSCRIBING", "HANDLING_COMMAND", "SPEAKING", "LISTENING", "STOPPED"}
	if got := states(logs); !slices.Equal(got, want) {
		t.Errorf("transitions %v, want %v", got, want)
	}
	if strings.Contains(logs, "¿Qué hora es?") {
		t.Error("the command text is in the info logs")
	}
}

func TestRunLogsTranscriptsWhenAsked(t *testing.T) {
	f := newFake(request{text: " ¿Qué hora es? "}, request{text: ""})
	var logs bytes.Buffer
	p := New(f.steps(), slog.New(slog.NewTextHandler(&logs, nil)))
	p.LogTranscripts = true
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	select {
	case <-f.finished:
		cancel()
		<-done
	case <-time.After(3 * time.Second):
		t.Fatal("pipeline did not finish the script")
	}
	for _, want := range []string{`level=INFO msg=heard text="¿Qué hora es?"`, `level=INFO msg=heard text=""`} {
		if !strings.Contains(logs.String(), want) {
			t.Errorf("logs have no %s:\n%s", want, logs.String())
		}
	}
}

func TestRunRecoversFromFailedRequests(t *testing.T) {
	sttDown := errors.New("whisper down")
	f := newFake(
		request{transcribeErr: sttDown},
		request{text: "  "},
		request{noSpeech: true},
		request{text: "a", sayErr: errors.New("speaker gone")},
		request{text: "b"},
	)
	if _, err := runScript(t, f); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error %v, want context.Canceled: a failed request must not stop the pipeline", err)
	}
	wantSaid := []string{ReplyServiceUnavailable, ReplyNotUnderstood, ReplyNoSpeech, "reply to a", "reply to b"}
	if !slices.Equal(f.said, wantSaid) {
		t.Errorf("said %q, want %q", f.said, wantSaid)
	}
	if !slices.Equal(f.handled, []string{"a", "b"}) {
		t.Errorf("handled %q, want only the recognized commands", f.handled)
	}
}

func TestRunDiscardsInputBeforeEachWait(t *testing.T) {
	f := newFake(request{text: "a"}, request{text: "b"})
	if _, err := runScript(t, f); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error %v", err)
	}
	for i, c := range f.calls {
		if c == "wait" && (i == 0 || f.calls[i-1] != "discard") {
			t.Fatalf("calls %v: wait at %d without a discard before it", f.calls, i)
		}
	}
}

func TestRunPlaysWakeSoundBeforeRecording(t *testing.T) {
	f := newFake(request{text: "a"})
	steps := f.steps()
	steps.Acknowledge = func(context.Context) error { f.call("ack"); return nil }
	runSteps(t, f, steps)
	want := []string{"discard", "wait", "ack", "discard", "record"}
	if got := f.calls[:len(want)]; !slices.Equal(got, want) {
		t.Errorf("calls %v, want %v: the sound plays, then its echo is discarded, then the recording starts", got, want)
	}
}

func TestRunGoesOnWhenWakeSoundFails(t *testing.T) {
	f := newFake(request{text: "a"})
	steps := f.steps()
	steps.Acknowledge = func(context.Context) error { return errors.New("no output device") }
	logs := runSteps(t, f, steps)
	if !slices.Equal(f.said, []string{"reply to a"}) {
		t.Errorf("said %q, want the reply despite the failed sound", f.said)
	}
	if !strings.Contains(logs, "wake sound failed") {
		t.Error("the failed sound is not logged")
	}
}

// runSteps is runScript with changed steps.
func runSteps(t *testing.T, f *fake, steps Steps) string {
	t.Helper()
	var logs bytes.Buffer
	p := New(steps, slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	select {
	case <-f.finished:
		cancel()
		if err := <-done; !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error %v", err)
		}
	case err := <-done:
		t.Fatalf("Run stopped early: %v", err)
	case <-time.After(3 * time.Second):
		t.Fatal("pipeline did not finish the script")
	}
	return logs.String()
}

func TestRunHandlesEachCommandOnce(t *testing.T) {
	f := newFake(request{text: "a"}, request{text: "a"}, request{text: "b"})
	if _, err := runScript(t, f); !errors.Is(err, context.Canceled) {
		t.Fatalf("Run error %v", err)
	}
	var records, handles int
	for _, c := range f.calls {
		switch c {
		case "record":
			records++
		case "handle":
			handles++
		}
	}
	if records != 3 || handles != 3 {
		t.Errorf("%d recordings and %d handled commands, want one handle per recording", records, handles)
	}
}

func TestRunStopsOnFatalErrors(t *testing.T) {
	micGone := errors.New("microphone gone")
	f := newFake(request{text: "a"})
	f.recErr = micGone
	if _, err := runScript(t, f); !errors.Is(err, micGone) {
		t.Errorf("Run error %v, want the recording error", err)
	}

	f = newFake()
	f.waitErr = wakeword.ErrUnknownModel
	if _, err := runScript(t, f); !errors.Is(err, wakeword.ErrUnknownModel) {
		t.Errorf("Run error %v, want the wake-word error", err)
	}
}

func TestRunStopsWhileSpeakingWithoutLeaks(t *testing.T) {
	before := runtime.NumGoroutine()
	f := newFake(request{text: "a"})
	f.hangSay = true
	var logs bytes.Buffer
	p := New(f.steps(), slog.New(slog.NewTextHandler(&logs, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- p.Run(ctx) }()
	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("Run error %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("Run did not stop while speaking")
	}
	deadline := time.Now().Add(time.Second)
	for runtime.NumGoroutine() > before && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if n := runtime.NumGoroutine(); n > before {
		t.Errorf("%d goroutines after Run, %d before", n, before)
	}
}
