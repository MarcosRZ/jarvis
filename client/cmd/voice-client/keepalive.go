package main

import (
	"log/slog"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/config"
)

// openKeepAlive opens the output device for the whole run and leaves it silent.
// An idle Bluetooth sink suspends after a few seconds; without this stream the
// first beep is lost while the sink resumes. The device callback fills the
// output with silence, so the sink stays awake while the assistant runs.
//
// A failure only disables the keepalive: the assistant keeps working, but a
// suspended sink may clip the start of the beep and the reply.
func openKeepAlive(cfg config.Config, backend *audio.Backend, logger *slog.Logger) *audio.Playback {
	if !cfg.KeepOutputAwake {
		return nil
	}
	playback, err := backend.OpenPlayback(cfg.OutputDevice, audio.BeepFormat, playbackBuffer)
	if err != nil {
		logger.Warn("output keepalive disabled", "error", err)
		return nil
	}
	return playback
}
