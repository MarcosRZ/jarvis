package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/config"
	"jarvis/client/internal/tts"
)

const flagText = "text"

// texts is a repeatable --text flag.
type texts []string

func (t *texts) String() string { return strings.Join(*t, " | ") }

func (t *texts) Set(v string) error {
	*t = append(*t, v)
	return nil
}

func (t texts) validate() error {
	if len(t) == 0 {
		return fmt.Errorf("--%s is missing: give the text to speak", flagText)
	}
	for _, text := range t {
		if strings.TrimSpace(text) == "" {
			return fmt.Errorf("--%s %q is empty", flagText, text)
		}
	}
	return nil
}

// say speaks each text in order through Piper and the output device.
func say(ctx context.Context, cfg config.Config, lines texts, stdout io.Writer, logger *slog.Logger) int {
	backend, err := audio.Open(logger)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()
	speaker := newSpeaker(cfg, backend, logger)
	if err := speaker.Check(ctx); err != nil {
		logger.Error("voice settings", "error", err)
		return exitError
	}

	failed := false
	for i, text := range lines {
		timing, err := speaker.Say(ctx, text)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return exitOK
			}
			logger.Error("speech failed", "error", err)
			fmt.Fprintf(stdout, "%d: failed\n", i+1)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "%d: %v of speech, Piper %v, playback %v\n", i+1,
			timing.Audio.Round(10*time.Millisecond), timing.Synthesis.Round(time.Millisecond), timing.Playback.Round(time.Millisecond))
	}
	if failed {
		return exitError
	}
	return exitOK
}

func newSpeaker(cfg config.Config, backend *audio.Backend, logger *slog.Logger) *tts.Speaker {
	client := tts.New(tts.Config{Addr: cfg.TTSAddr, Voice: cfg.TTSVoice, Speaker: cfg.TTSSpeaker, Timeouts: timeouts(cfg)})
	return tts.NewSpeaker(client, func(format audio.Format) (tts.Player, error) {
		playback, err := backend.OpenPlayback(cfg.OutputDevice, format, playbackBuffer)
		if err != nil {
			return nil, err
		}
		logStreamFormat(logger, "playing", playback.Format(), playback.NativeFormat())
		return playback, nil
	})
}
