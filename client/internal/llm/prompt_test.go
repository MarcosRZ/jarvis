package llm

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestFetchPrompt(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method", http.StatusMethodNotAllowed)
			return
		}
		_, _ = w.Write([]byte(`{"prompt":"  custom prompt  "}`))
	}))
	t.Cleanup(srv.Close)

	got, err := FetchPrompt(context.Background(), srv.URL, time.Second)
	if err != nil {
		t.Fatalf("FetchPrompt: %v", err)
	}
	if got != "custom prompt" {
		t.Errorf("prompt %q, want %q", got, "custom prompt")
	}
}

func TestFetchPromptEmptyURL(t *testing.T) {
	got, err := FetchPrompt(context.Background(), "", time.Second)
	if err != nil || got != "" {
		t.Errorf("FetchPrompt = %q, %v; want empty and no error", got, err)
	}
}

func TestBuildQuestionPrompt(t *testing.T) {
	got := buildQuestionPrompt("", "weather.casa: rain")
	for _, want := range []string{"helpful voice assistant", "general questions", "weather.casa: rain", `"reply"`} {
		if !strings.Contains(got, want) {
			t.Errorf("default prompt does not contain %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, ContextPlaceholder) {
		t.Errorf("default prompt still contains the placeholder:\n%s", got)
	}

	got = buildQuestionPrompt("You are a custom assistant.", "light.kitchen: on")
	for _, want := range []string{"You are a custom assistant.", "light.kitchen: on", `"reply"`} {
		if !strings.Contains(got, want) {
			t.Errorf("custom prompt does not contain %q:\n%s", want, got)
		}
	}

	got = buildQuestionPrompt("Use this context:\n"+ContextPlaceholder, "sensor.casa: 42")
	if !strings.Contains(got, "sensor.casa: 42") || strings.Contains(got, ContextPlaceholder) {
		t.Errorf("placeholder was not replaced:\n%s", got)
	}
}

func TestInterpretAnswerUsesCustomPrompt(t *testing.T) {
	requests := make(chan request, 1)
	srv := testServer(t, `{"reply":"Un chiste corto."}`, requests)
	c := New(Config{URL: srv.URL, Timeout: time.Second, Prompt: "CUSTOM INSTRUCTION"})

	got, err := c.InterpretAnswer(context.Background(), "cuéntame un chiste", "light.kitchen: on")
	if err != nil {
		t.Fatalf("InterpretAnswer: %v", err)
	}
	if got != "Un chiste corto." {
		t.Errorf("reply %q", got)
	}
	system := (<-requests).Messages[0].Content
	for _, want := range []string{"CUSTOM INSTRUCTION", "light.kitchen: on", `"reply"`} {
		if !strings.Contains(system, want) {
			t.Errorf("system prompt does not contain %q:\n%s", want, system)
		}
	}
}
