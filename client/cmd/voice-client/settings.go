package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// settingsTimeout bounds the dashboard request at startup, so a stopped
// dashboard cannot delay the assistant.
const settingsTimeout = 2 * time.Second

type remoteSettings struct {
	Values map[string]string `json:"values"`
}

// fetchRemoteSettings reads the client overrides saved in the dashboard. It
// returns nil when VOICE_SETTINGS_URL is unset or off, or when the dashboard is
// unreachable. In every failure case the .env values remain in use.
func fetchRemoteSettings(
	ctx context.Context,
	getenv func(string) string,
	stderr io.Writer,
) map[string]string {
	raw := strings.TrimSpace(getenv("VOICE_SETTINGS_URL"))
	if raw == "" || strings.EqualFold(raw, "off") || strings.EqualFold(raw, "none") {
		return nil
	}
	requestCtx, cancel := context.WithTimeout(ctx, settingsTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(requestCtx, http.MethodGet, raw, nil)
	if err != nil {
		fmt.Fprintf(stderr, "remote settings: %v\n", err)
		return nil
	}
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		fmt.Fprintf(stderr, "remote settings unavailable: %v\n", err)
		return nil
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		fmt.Fprintf(stderr, "remote settings unavailable: %s\n", response.Status)
		return nil
	}
	var body remoteSettings
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&body); err != nil {
		fmt.Fprintf(stderr, "remote settings: %v\n", err)
		return nil
	}
	values := make(map[string]string, len(body.Values))
	for key, value := range body.Values {
		// The endpoint is local but the client only accepts environment keys
		// of its own namespace.
		if strings.HasPrefix(key, "VOICE_") && value != "" {
			values[key] = value
		}
	}
	if len(values) == 0 {
		return nil
	}
	return values
}
