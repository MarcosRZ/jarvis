package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/config"
	"jarvis/client/internal/wakeword"
	"jarvis/client/internal/wyoming"
)

const clockFormat = "15:04:05.000"

func timeouts(cfg config.Config) wyoming.Timeouts {
	return wyoming.Timeouts{Connect: cfg.ConnectTimeout, Operation: cfg.OperationTimeout}
}

func newDetector(cfg config.Config, format audio.Format, logger *slog.Logger) *wakeword.Detector {
	return wakeword.New(wakeword.Config{
		Addr:     cfg.WakeWordAddr,
		Model:    cfg.WakeWordModel,
		Cooldown: cfg.WakeWordCooldown,
		Timeouts: timeouts(cfg),
		Logger:   logger,
	}, format)
}

// wakeTest streams the microphone to openWakeWord and prints each detection
// until Ctrl+C. It uses no other voice service.
func wakeTest(ctx context.Context, cfg config.Config, stdout io.Writer, logger *slog.Logger) int {
	backend, capture, err := openMicrophone(cfg.InputDevice, logger)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()
	defer capture.Close()

	detector := newDetector(cfg, capture.Format(), logger)
	detections := make(chan wakeword.Detection)
	done := make(chan error, 1)
	go func() { done <- detector.Run(ctx, capture, detections) }()

	started := time.Now()
	fmt.Fprintf(stdout, "say the wake word (%s); press Ctrl+C to stop\n", cfg.WakeWordModel)
	count := 0
	for {
		select {
		case det := <-detections:
			count++
			fmt.Fprintf(stdout, "%s  detected %s  latency %v\n", det.At.Format(clockFormat), det.Model, det.Latency.Round(time.Millisecond))
		case err := <-done:
			fmt.Fprintf(stdout, "%d detections in %v, dropped %d bytes of audio\n", count, time.Since(started).Round(time.Second), capture.Dropped())
			if errors.Is(err, context.Canceled) {
				return exitOK
			}
			logger.Error("wake-word detection stopped", "error", err)
			return exitError
		}
	}
}
