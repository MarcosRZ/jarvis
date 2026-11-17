package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
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
	"jarvis/client/internal/textinput"
	"jarvis/client/internal/wakeword"
)

const (
	// haTimeout bounds one Home Assistant request, so a slow Home Assistant
	// delays a spoken reply by at most this much.
	haTimeout     = 5 * time.Second
	deviceRefresh = 30 * time.Second
	promptTimeout = 2 * time.Second

	// The learning publisher checks for changes often, polls the dashboard for
	// queued edits, sends a heartbeat so the dashboard can tell a quiet client
	// from a stopped one, and gives the dashboard a short timeout so a failure
	// never touches the voice pipeline.
	learningPublishCheck   = 5 * time.Second
	learningOperationCheck = 2 * time.Second
	learningHeartbeat      = 60 * time.Second
	learningPublishTimeout = 3 * time.Second
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
	var learningStore *learning.Store
	if cfg.LearningFile != "" {
		store, err := learning.Open(cfg.LearningFile)
		if err != nil {
			logger.Warn("learning disabled", "file", cfg.LearningFile, "error", err)
		} else {
			handler.UseLearning(store)
			learningStore = store
			logger.Info("learning", "file", cfg.LearningFile, "entries", store.Len())
		}
	}
	if learningStore != nil && cfg.LearningURL != "" {
		go publishLearning(ctx, learningStore, cfg.LearningURL, logger)
	}
	// A guess has words of the command, which are private like the transcript.
	guessLevel := slog.LevelDebug
	if cfg.LogTranscripts {
		guessLevel = slog.LevelInfo
	}
	handler.UseMatching(cfg.Matching, logger, guessLevel)
	handler.UseRoom(cfg.Room)
	handler.UseInference(cfg.LLMOrder, cfg.LLMSafety)
	if cfg.LLMMode != commands.InterpretOff {
		prompt := ""
		if cfg.PromptURL != "" {
			loaded, err := llm.FetchPrompt(ctx, cfg.PromptURL, promptTimeout)
			if err != nil {
				logger.Warn("language model prompt", "url", cfg.PromptURL, "error", err)
			} else if loaded != "" {
				prompt = loaded
				logger.Info("language model prompt", "source", "dashboard", "url", cfg.PromptURL)
			}
		}
		interpreter := llm.New(llm.Config{URL: cfg.LLMURL, Timeout: cfg.LLMTimeout, Room: cfg.Room, Prompt: prompt})
		handler.UseInterpreter(interpreter, cfg.LLMMode)
		logger.Info("language model", "url", cfg.LLMURL, "mode", cfg.LLMMode, "safety", cfg.LLMSafety, "order", cfg.LLMOrder, "room", cfg.Room)
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
	if cfg.TextAddr != "" {
		server := textinput.NewServer(cfg.TextAddr, p.Submit, logger)
		if err := server.Start(ctx); err != nil {
			logger.Error("text input", "error", err)
			return exitError
		}
		logger.Info("text input", "addr", cfg.TextAddr)
	}
	logger.Info("assistant started", "version", version, "wake_word_model", cfg.WakeWordModel, "voice", cfg.TTSVoice, "config_file", cfg.ConfigFile, "beep", cfg.BeepEnabled)
	return stopCode(p.Run(ctx), logger)
}

// newCommandHandler returns a handler with no devices, or, with --ha-url, one
// that controls the lights and switches exposed in Home Assistant, reads that
// list again every deviceRefresh, and sends every other sentence to the Home
// Assistant conversation agent.
func newCommandHandler(ctx context.Context, cfg config.Config, logger *slog.Logger) (*commands.Handler, error) {
	if cfg.HAURL == "" {
		return commands.New(time.Now, nil, nil)
	}
	ha := homeassistant.New(homeassistant.Config{
		URL:                 cfg.HAURL,
		Token:               cfg.HAToken,
		Timeout:             haTimeout,
		ConversationTimeout: cfg.OperationTimeout,
		Language:            cfg.STTLanguage,
		AgentID:             cfg.HAAgent,
	})
	handler, err := commands.New(time.Now, nil, ha)
	if err != nil {
		return nil, err
	}
	handler.UseSource(ha, deviceRefresh, func(err error) { logger.Warn("Home Assistant devices", "error", err) })
	handler.UseActions(nil, ha)
	handler.UseActionSource(ha, deviceRefresh, func(err error) { logger.Warn("Home Assistant actions", "error", err) })
	handler.UseConversation(ha)
	handler.UseContext(ha)
	if err := handler.Refresh(ctx); err != nil {
		if errors.Is(err, homeassistant.ErrUnauthorized) {
			return nil, err
		}
		logger.Warn("Home Assistant not available; retrying at the next command", "error", err)
	}
	if err := handler.RefreshActions(ctx); err != nil {
		if errors.Is(err, homeassistant.ErrUnauthorized) {
			return nil, err
		}
		logger.Warn("Home Assistant actions not available; retrying at the next command", "error", err)
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
	logger.Info("Home Assistant actions", "url", cfg.HAURL, "count", len(handler.Actions()))
	return handler, nil
}

// publishLearning mirrors the learned associations to the dashboard without
// blocking the voice pipeline, and applies the edits queued from the Learning
// page. It sends at startup, after every change, after applying an operation
// and on a heartbeat, so the dashboard can tell a quiet client from a stopped
// one. A repeated failure is logged once until the call succeeds again.
func publishLearning(ctx context.Context, store *learning.Store, url string, logger *slog.Logger) {
	client := &http.Client{Timeout: learningPublishTimeout}
	check := time.NewTicker(learningPublishCheck)
	defer check.Stop()
	operations := time.NewTicker(learningOperationCheck)
	defer operations.Stop()
	operationsURL := learning.OperationsURL(url)

	var lastRevision uint64
	var publishedAt time.Time
	var appliedThrough int64
	var publishedThrough int64
	var lastError string
	var lastOperationError string

	publish := func() {
		// Read the revision before the snapshot: a change in between causes one
		// extra publish instead of losing the change until the next heartbeat.
		revision := store.Revision()
		snapshot := store.Snapshot(time.Now())
		snapshot.AppliedThrough = appliedThrough
		if err := learning.Publish(ctx, client, url, snapshot); err != nil {
			if message := err.Error(); message != lastError {
				logger.Warn("learning snapshot", "url", url, "error", err)
				lastError = message
			}
			return
		}
		lastRevision = revision
		publishedAt = time.Now()
		publishedThrough = appliedThrough
		lastError = ""
		logger.Debug("learning snapshot", "url", url, "entries", len(snapshot.Entries))
	}

	apply := func() bool {
		queued, err := learning.FetchOperations(ctx, client, operationsURL, appliedThrough)
		if err != nil {
			if message := err.Error(); message != lastOperationError {
				logger.Warn("learning operations", "url", operationsURL, "error", err)
				lastOperationError = message
			}
			return false
		}
		lastOperationError = ""
		changed := false
		for _, op := range queued {
			if op.ID <= appliedThrough {
				continue
			}
			applied, err := store.ApplyOperation(op, time.Now())
			if err != nil {
				if errors.Is(err, learning.ErrInvalidOperation) {
					logger.Warn("learning operation rejected", "id", op.ID, "kind", op.Kind, "error", err)
					appliedThrough = op.ID
					changed = true
					continue
				}
				logger.Warn("learning operation not applied; will retry", "id", op.ID, "kind", op.Kind, "error", err)
				break
			}
			appliedThrough = op.ID
			changed = true
			if applied {
				logger.Debug("learning operation applied", "id", op.ID, "kind", op.Kind, "entries", store.Len())
			}
		}
		return changed
	}

	publish()
	for {
		select {
		case <-ctx.Done():
			return
		case <-operations.C:
			if apply() || appliedThrough != publishedThrough {
				publish()
			}
		case <-check.C:
			if store.Revision() != lastRevision || appliedThrough != publishedThrough || time.Since(publishedAt) >= learningHeartbeat {
				publish()
			}
		}
	}
}
