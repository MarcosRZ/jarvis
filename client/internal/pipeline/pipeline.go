// Package pipeline runs the assistant loop: wake word, recording,
// transcription, command, spoken reply. One goroutine runs all states, so
// requests never overlap.
package pipeline

import (
	"context"
	"log/slog"
	"strings"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/stt"
	"jarvis/client/internal/tts"
	"jarvis/client/internal/wakeword"
)

type State string

const (
	StateStarting     State = "STARTING"
	StateListening    State = "LISTENING"
	StateRecording    State = "RECORDING"
	StateTranscribing State = "TRANSCRIBING"
	StateHandling     State = "HANDLING_COMMAND"
	StateSpeaking     State = "SPEAKING"
	StateStopped      State = "STOPPED"
)

const (
	ReplyNoSpeech           = "No te he oído."
	ReplyNotUnderstood      = "No te he entendido."
	ReplyServiceUnavailable = "Ahora no puedo entenderte. El servicio de voz no responde."
)

// Steps are the adapters of the pipeline. WaitForWakeWord and Record errors
// stop the pipeline; they come from the microphone or the configuration.
type Steps struct {
	WaitForWakeWord func(ctx context.Context) (wakeword.Detection, error)
	// Acknowledge plays the wake-word sound, if not nil. A failure is logged
	// and the request goes on without the sound.
	Acknowledge func(ctx context.Context) error
	Record      func(ctx context.Context) (audio.Recording, error)
	Transcribe  func(ctx context.Context, format audio.Format, pcm []byte) (stt.Result, error)
	Handle      func(ctx context.Context, text string) string
	Say         func(ctx context.Context, text string) (tts.Timing, error)
	// DiscardInput drops buffered microphone audio before listening again, so
	// the assistant's own voice is never taken as a wake word or a command.
	DiscardInput func()
}

type Pipeline struct {
	// LogTranscripts logs what Whisper heard at info level. Off, it is logged
	// only at debug level, because the text can be private.
	LogTranscripts bool

	steps   Steps
	logger  *slog.Logger
	state   State
	entered time.Time
}

func New(steps Steps, logger *slog.Logger) *Pipeline {
	return &Pipeline{steps: steps, logger: logger, state: StateStarting, entered: time.Now()}
}

// Run serves requests until ctx ends or a step fails in a way that a retry
// cannot fix. A failed request gets a spoken error and does not stop Run.
func (p *Pipeline) Run(ctx context.Context) error {
	for {
		if err := p.serve(ctx); err != nil {
			p.enter(StateStopped, "reason", err)
			return err
		}
	}
}

// serve handles one wake word, from listening to the end of the reply.
func (p *Pipeline) serve(ctx context.Context) error {
	p.steps.DiscardInput()
	p.enter(StateListening)
	det, err := p.steps.WaitForWakeWord(ctx)
	if err != nil {
		return err
	}

	if p.steps.Acknowledge != nil {
		if err := p.steps.Acknowledge(ctx); err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			p.logger.Warn("wake sound failed", "error", err)
		}
		// The microphone heard the sound; it must not start the recording.
		p.steps.DiscardInput()
	}

	p.enter(StateRecording, "wake_latency", det.Latency)
	rec, err := p.steps.Record(ctx)
	if err != nil {
		return err
	}
	if rec.End == audio.EndNoSpeech {
		return p.reply(ctx, ReplyNoSpeech, time.Now())
	}

	p.enter(StateTranscribing, "recorded", rec.Format.Duration(len(rec.PCM)), "end", rec.End)
	recorded := time.Now()
	res, err := p.steps.Transcribe(ctx, rec.Format, rec.PCM)
	clear(rec.PCM)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.logger.Error("transcription failed", "error", err)
		return p.reply(ctx, ReplyServiceUnavailable, recorded)
	}
	text := strings.TrimSpace(res.Text)
	level := slog.LevelDebug
	if p.LogTranscripts {
		level = slog.LevelInfo
	}
	p.logger.Log(ctx, level, "heard", "text", text)
	if text == "" {
		return p.reply(ctx, ReplyNotUnderstood, recorded)
	}

	p.enter(StateHandling, "stt_processing", res.Processing, "stt_total", res.Total)
	return p.reply(ctx, p.steps.Handle(ctx, text), recorded)
}

// reply speaks text. since is the end of the recording, for the response latency.
func (p *Pipeline) reply(ctx context.Context, text string, since time.Time) error {
	p.enter(StateSpeaking)
	untilSpeech := time.Since(since)
	timing, err := p.steps.Say(ctx, text)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		p.logger.Error("speech failed", "error", err)
		return nil
	}
	attrs := []any{
		"response_latency", (untilSpeech + timing.Synthesis).Round(time.Millisecond),
		"synthesis", timing.Synthesis.Round(time.Millisecond),
		"audio", timing.Audio.Round(10 * time.Millisecond),
		"playback", timing.Playback.Round(time.Millisecond),
	}
	if p.LogTranscripts {
		attrs = append(attrs, "text", text)
	}
	p.logger.Info("spoke", attrs...)
	return nil
}

func (p *Pipeline) enter(next State, attrs ...any) {
	now := time.Now()
	attrs = append([]any{"from", p.state, "to", next, "after", now.Sub(p.entered).Round(time.Millisecond)}, attrs...)
	p.logger.Info("state", attrs...)
	p.state, p.entered = next, now
}
