package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/websocket"
)

const maxWebSocketBytes = 8 << 20

// wsMessage is the subset of the Home Assistant WebSocket protocol that the
// collector needs for the exposure list.
type wsMessage struct {
	ID      int             `json:"id"`
	Type    string          `json:"type"`
	Success bool            `json:"success"`
	Result  json.RawMessage `json:"result"`
	Error   *struct {
		Code    string `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

// ExposedEntities returns the entities exposed to each Home Assistant voice
// assistant. The exposure settings are only available over the WebSocket API,
// so the collector cannot derive the action catalog from /api/states alone.
func (c *homeAssistantClient) ExposedEntities(ctx context.Context) (map[string]map[string]bool, error) {
	url := "ws" + strings.TrimPrefix(strings.TrimRight(c.baseURL, "/"), "http") + "/api/websocket"
	conn, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Home Assistant WebSocket %s: %w", url, err)
	}
	defer conn.CloseNow()
	conn.SetReadLimit(maxWebSocketBytes)

	var message wsMessage
	if err := readWSMessage(ctx, conn, &message); err != nil || message.Type != "auth_required" {
		return nil, fmt.Errorf("Home Assistant WebSocket: no auth_required message (%v)", err)
	}
	if err := writeWSMessage(ctx, conn, map[string]any{"type": "auth", "access_token": c.token}); err != nil {
		return nil, err
	}
	if err := readWSMessage(ctx, conn, &message); err != nil {
		return nil, err
	}
	switch message.Type {
	case "auth_ok":
	case "auth_invalid":
		return nil, fmt.Errorf("home assistant rejected the token")
	default:
		return nil, fmt.Errorf("Home Assistant WebSocket: unexpected %q after auth", message.Type)
	}

	if err := writeWSMessage(ctx, conn, map[string]any{
		"id":   1,
		"type": "homeassistant/expose_entity/list",
	}); err != nil {
		return nil, err
	}
	for {
		var result wsMessage
		if err := readWSMessage(ctx, conn, &result); err != nil {
			return nil, err
		}
		if result.ID != 1 || result.Type != "result" {
			continue
		}
		if !result.Success {
			if result.Error != nil {
				return nil, fmt.Errorf("Home Assistant expose_entity/list: %s: %s", result.Error.Code, result.Error.Message)
			}
			return nil, fmt.Errorf("Home Assistant expose_entity/list failed")
		}
		var payload struct {
			ExposedEntities map[string]map[string]bool `json:"exposed_entities"`
		}
		if err := json.Unmarshal(result.Result, &payload); err != nil {
			return nil, err
		}
		if payload.ExposedEntities == nil {
			payload.ExposedEntities = map[string]map[string]bool{}
		}
		return payload.ExposedEntities, nil
	}
}

func writeWSMessage(ctx context.Context, conn *websocket.Conn, value any) error {
	body, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return conn.Write(ctx, websocket.MessageText, body)
}

func readWSMessage(ctx context.Context, conn *websocket.Conn, value any) error {
	_, body, err := conn.Read(ctx)
	if err != nil {
		return fmt.Errorf("Home Assistant WebSocket: %w", err)
	}
	return json.Unmarshal(body, value)
}
