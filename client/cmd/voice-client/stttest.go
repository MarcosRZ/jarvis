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
	"jarvis/client/internal/stt"
)

// sttTest waits for the wake word, records one command, prints what Whisper
// recognized, and repeats until Ctrl+C. The recording is held in memory only
// and cleared after the transcription.
func sttTest(ctx context.Context, cfg config.Config, stdout io.Writer, logger *slog.Logger) int {
	backend, capture, err := openMicrophone(cfg.InputDevice, logger)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()
	defer capture.Close()

	detector := newDetector(cfg, capture.Format(), logger)
	client := stt.New(stt.Config{Addr: cfg.STTAddr, Language: cfg.STTLanguage, Timeouts: timeouts(cfg)})
	limits := recordLimits(cfg)

	fmt.Fprintf(stdout, "say the wake word (%s), then a command; press Ctrl+C to stop\n", cfg.WakeWordModel)
	for {
		det, err := detector.Wait(ctx, capture)
		if err != nil {
			return stopCode(err, logger)
		}
		fmt.Fprintf(stdout, "%s  wake word (latency %v), listening\n", det.At.Format(clockFormat), det.Latency.Round(time.Millisecond))

		rec, err := audio.Record(ctx, capture, capture.Format(), limits)
		if err != nil {
			return stopCode(err, logger)
		}
		if rec.End == audio.EndNoSpeech {
			fmt.Fprintf(stdout, "    no speech within %v\n", limits.StartTimeout)
			continue
		}
		length := rec.Format.Duration(len(rec.PCM)).Round(10 * time.Millisecond)
		res, err := client.Transcribe(ctx, rec.Format, rec.PCM)
		clear(rec.PCM)
		if err != nil {
			if ctx.Err() != nil {
				return exitOK
			}
			logger.Error("transcription failed", "error", err)
			fmt.Fprintf(stdout, "    recorded %v (end: %s), transcription failed\n", length, rec.End)
			continue
		}
		text := fmt.Sprintf("%q", res.Text)
		if res.Text == "" {
			text = "nothing recognized"
		}
		fmt.Fprintf(stdout, "    recorded %v (end: %s), Whisper %v, total %v: %s\n",
			length, rec.End, res.Processing.Round(time.Millisecond), res.Total.Round(time.Millisecond), text)
	}
}

func recordLimits(cfg config.Config) audio.RecordLimits {
	return audio.RecordLimits{
		Max:          cfg.RecordMax,
		StartTimeout: cfg.RecordStartTimeout,
		EndSilence:   cfg.RecordEndSilence,
		SpeechLevel:  cfg.SpeechLevel,
	}
}

func stopCode(err error, logger *slog.Logger) int {
	if errors.Is(err, context.Canceled) {
		return exitOK
	}
	logger.Error("stopped", "error", err)
	return exitError
}
