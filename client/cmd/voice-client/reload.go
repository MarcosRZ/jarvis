package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"os"
	"time"

	"jarvis/client/internal/config"
	"jarvis/client/internal/llm"
	"jarvis/client/internal/news"
)

// settingsPoll is how often the client rereads the dashboard overrides. A
// saved model configuration applies to the next request without a restart.
const settingsPoll = 30 * time.Second

// pollSettings applies the model and news overrides saved in Manhattan. A
// failed lookup keeps the last known configuration, and an unchanged value is
// not applied again.
func pollSettings(ctx context.Context, base config.Config, prompt string, model *llm.Client, headlines *news.Client, logger *slog.Logger) {
	ticker := time.NewTicker(settingsPoll)
	defer ticker.Stop()
	applied := ""
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			values := fetchRemoteSettings(ctx, os.Getenv, io.Discard)
			if values == nil {
				continue
			}
			next, newsURL, signature := settingsConfig(base, prompt, values)
			if signature == applied {
				continue
			}
			if model != nil {
				model.Update(next)
			}
			if headlines != nil {
				headlines.Update(newsURL, 0, 0)
			}
			applied = signature
			logger.Info("settings applied", "model_url", next.URL, "model", next.Model, "timeout", next.Timeout)
		}
	}
}

// settingsConfig builds the model configuration from the startup values and
// the saved overrides. An override that is absent keeps the startup value.
func settingsConfig(base config.Config, prompt string, values map[string]string) (llm.Config, string, string) {
	url, model, key := base.LLMURL, base.LLMModel, base.LLMAPIKey
	timeout := base.LLMTimeout
	newsURL := base.NewsURL
	if value, ok := values[config.EnvLLMURL]; ok && value != "" {
		url = value
	}
	if value, ok := values[config.EnvLLMModel]; ok && value != "" {
		model = value
	}
	if value, ok := values[config.EnvLLMAPIKey]; ok {
		key = value
	}
	if value, ok := values[config.EnvLLMTimeout]; ok && value != "" {
		if parsed, err := time.ParseDuration(value); err == nil && parsed > 0 {
			timeout = parsed
		}
	}
	if value, ok := values[config.EnvNewsURL]; ok && value != "" {
		newsURL = value
	}
	next := llm.Config{
		URL:     url,
		Model:   model,
		APIKey:  key,
		Timeout: timeout,
		Room:    base.Room,
		Prompt:  prompt,
	}
	signature := fmt.Sprintf("%s|%s|%s|%v|%s|%d", url, model, key, timeout, newsURL, len(key))
	return next, newsURL, signature
}
