package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"
)

type homeAssistantClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type haState struct {
	EntityID   string                     `json:"entity_id"`
	State      string                     `json:"state"`
	Attributes map[string]json.RawMessage `json:"attributes"`
}

type homeAssistantResult struct {
	Status        SourceStatus
	Reachable     bool
	HostUptime    *int64
	HomeAssistant *Service
}

func newHomeAssistantClient(cfg homeAssistantConfig, timeout time.Duration) *homeAssistantClient {
	if cfg.URL == "" || cfg.Token == "" {
		return nil
	}
	return &homeAssistantClient{
		baseURL: strings.TrimRight(cfg.URL, "/"),
		token:   cfg.Token,
		client:  &http.Client{Timeout: timeout},
	}
}

func (c *homeAssistantClient) States(ctx context.Context) ([]haState, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/states", nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Authorization", "Bearer "+c.token)
	response, err := c.client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return nil, fmt.Errorf("home assistant rejected the token")
	}
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("home assistant returned %s", response.Status)
	}
	var states []haState
	if err := json.NewDecoder(response.Body).Decode(&states); err != nil {
		return nil, err
	}
	return states, nil
}

func hostUptimeFromStates(states []haState, now time.Time) *int64 {
	for _, state := range states {
		entity := strings.ToLower(state.EntityID)
		if strings.Contains(entity, "last_boot") {
			if boot, err := time.Parse(time.RFC3339, state.State); err == nil {
				seconds := int64(now.Sub(boot).Seconds())
				if seconds >= 0 {
					return &seconds
				}
			}
		}
	}
	for _, state := range states {
		entity := strings.ToLower(state.EntityID)
		if !strings.Contains(entity, "uptime") {
			continue
		}
		value, err := strconv.ParseFloat(state.State, 64)
		if err != nil || value < 0 {
			continue
		}
		unit := ""
		if raw, ok := state.Attributes["unit_of_measurement"]; ok {
			_ = json.Unmarshal(raw, &unit)
		}
		seconds := int64(value)
		switch unit {
		case "h", "hours":
			seconds = int64(value * 3600)
		case "d", "days":
			seconds = int64(value * 86400)
		}
		return &seconds
	}
	return nil
}
