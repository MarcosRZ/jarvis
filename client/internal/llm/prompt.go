package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// ContextPlaceholder is replaced with the read-only Home Assistant context.
	ContextPlaceholder = "{{context}}"
	maxPromptBytes     = 32 << 10
)

// Keep in sync with dashboard/src/lib/default-prompt.ts.
const defaultQuestionPrompt = `You are a helpful voice assistant. Answer in the language of the user, in one or two short sentences.
You may answer general questions from your own knowledge, including jokes, explanations, and conversation.
Use the Home Assistant context when it is relevant. Do not invent device states, sensor values, or forecasts.
If you cannot answer, return an empty reply.

Home Assistant context (read-only):
{{context}}
`

// FetchPrompt reads the configurable prompt from a dashboard endpoint. An empty
// URL returns an empty prompt, which means the built-in prompt is used.
func FetchPrompt(ctx context.Context, url string, timeout time.Duration) (string, error) {
	if strings.TrimSpace(url) == "" {
		return "", nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("prompt: HTTP %d", resp.StatusCode)
	}
	var payload struct {
		Prompt string `json:"prompt"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxPromptBytes)).Decode(&payload); err != nil {
		return "", fmt.Errorf("prompt: %w", err)
	}
	return strings.TrimSpace(payload.Prompt), nil
}

func buildQuestionPrompt(custom, homeContext string) string {
	homeContext = strings.TrimSpace(homeContext)
	if homeContext == "" {
		homeContext = "(none)"
	}
	prompt := custom
	if prompt == "" {
		prompt = strings.ReplaceAll(defaultQuestionPrompt, ContextPlaceholder, homeContext)
	} else {
		prompt = strings.TrimSpace(prompt)
		if strings.Contains(prompt, ContextPlaceholder) {
			prompt = strings.ReplaceAll(prompt, ContextPlaceholder, homeContext)
		} else {
			prompt += "\n\nHome Assistant context (read-only):\n" + homeContext
		}
	}
	return prompt + "\n\nReturn one JSON object with the key \"reply\". Do not add text outside the JSON object.\n"
}
