package main

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/commands"
	"jarvis/client/internal/config"
	"jarvis/client/internal/homeassistant"
	"jarvis/client/internal/learning"
	"jarvis/client/internal/llm"
	"jarvis/client/internal/pipeline"
	"jarvis/client/internal/stt"
	"jarvis/client/internal/wakeword"
)

const (
	// haTimeout bounds one Home Assistant request, so a slow Home Assistant
	// delays a spoken reply by at most this much.
	haTimeout     = 5 * time.Second
	deviceRefresh = 30 * time.Second
)

// runAssistant connects the microphone, the voice services, the command
// handler and the speaker into the pipeline, and runs it until Ctrl+C.
func runAssistant(ctx context.Context, cfg config.Config, logger *slog.Logger) int {
	backend, capture, err := openMicrophone(cfg.InputDevice, logger)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()
	defer capture.Close()

	speaker := newSpeaker(cfg, backend, logger)
	if err := speaker.Check(ctx); err != nil {
		return stopCode(err, logger)
	}
	detector := newDetector(cfg, capture.Format(), logger)
	transcriber := stt.New(stt.Config{Addr: cfg.STTAddr, Language: cfg.STTLanguage, Timeouts: timeouts(cfg)})
	handler, err := newCommandHandler(ctx, cfg, logger)
	if err != nil {
		return stopCode(err, logger)
	}
	if cfg.LearningFile != "" {
		store, err := learning.Open(cfg.LearningFile)
		if err != nil {
			logger.Warn("learning disabled", "file", cfg.LearningFile, "error", err)
		} else {
			handler.UseLearning(store)
			logger.Info("learning", "file", cfg.LearningFile, "entries", store.Len())
		}
	}
	// A guess has words of the command, which are private like the transcript.
	guessLevel := slog.LevelDebug
	if cfg.LogTranscripts {
		guessLevel = slog.LevelInfo
	}
	handler.UseMatching(cfg.Matching, logger, guessLevel)
	handler.UseRoom(cfg.Room)
	if cfg.LLMMode != commands.InterpretOff {
		interpreter := llm.New(llm.Config{URL: cfg.LLMURL, Timeout: cfg.LLMTimeout, Room: cfg.Room})
		handler.UseInterpreter(interpreter, cfg.LLMMode)
		logger.Info("language model", "url", cfg.LLMURL, "mode", cfg.LLMMode, "room", cfg.Room)
	}
	limits := recordLimits(cfg)
	var acknowledge func(context.Context) error
	if cfg.BeepEnabled {
		beep := cfg.Beep.PCM()
		acknowledge = func(ctx context.Context) error {
			_, err := speaker.Play(ctx, audio.BeepFormat, beep)
			return err
		}
	}

	p := pipeline.New(pipeline.Steps{
		WaitForWakeWord: func(ctx context.Context) (wakeword.Detection, error) { return detector.Wait(ctx, capture) },
		Acknowledge:     acknowledge,
		Record: func(ctx context.Context) (audio.Recording, error) {
			return audio.Record(ctx, capture, capture.Format(), limits)
		},
		Transcribe:   transcriber.Transcribe,
		Handle:       handler.Handle,
		Say:          speaker.Say,
		DiscardInput: capture.Discard,
	}, logger)
	p.LogTranscripts = cfg.LogTranscripts
	logger.Info("assistant started", "version", version, "wake_word_model", cfg.WakeWordModel, "voice", cfg.TTSVoice, "config_file", cfg.ConfigFile, "beep", cfg.BeepEnabled)
	return stopCode(p.Run(ctx), logger)
}

// newCommandHandler returns a handler with no devices, or, with --ha-url, one
// that controls the lights and switches exposed in Home Assistant and reads
// that list again every deviceRefresh.
func newCommandHandler(ctx context.Context, cfg config.Config, logger *slog.Logger) (*commands.Handler, error) {
	if cfg.HAURL == "" {
		return commands.New(time.Now, nil, nil)
	}
	ha := homeassistant.New(homeassistant.Config{URL: cfg.HAURL, Token: cfg.HAToken, Timeout: haTimeout})
	handler, err := commands.New(time.Now, nil, ha)
	if err != nil {
		return nil, err
	}
	handler.UseSource(ha, deviceRefresh, func(err error) { logger.Warn("Home Assistant devices", "error", err) })
	if err := handler.Refresh(ctx); err != nil {
		if errors.Is(err, homeassistant.ErrUnauthorized) {
			return nil, err
		}
		logger.Warn("Home Assistant not available; retrying at the next command", "error", err)
	}
	names := make([]string, 0)
	for _, d := range handler.Devices() {
		if d.Unavailable {
			names = append(names, d.Name+" (unavailable)")
		} else {
			names = append(names, d.Name)
		}
	}
	logger.Info("Home Assistant devices", "url", cfg.HAURL, "count", len(names), "names", strings.Join(names, ", "))
	return handler, nil
}
