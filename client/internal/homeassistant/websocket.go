package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/coder/websocket"
)

// Session is an authenticated connection to the Home Assistant WebSocket API.
// Some data, such as the exposure settings, is only available there.
type Session struct {
	c      *websocket.Conn
	nextID int
}

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

// Connect opens a session and authenticates with cfg.Token.
func Connect(ctx context.Context, cfg Config) (*Session, error) {
	ctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	url := "ws" + strings.TrimPrefix(strings.TrimRight(cfg.URL, "/"), "http") + "/api/websocket"
	c, _, err := websocket.Dial(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("Home Assistant WebSocket %s: %w", url, err)
	}
	c.SetReadLimit(maxResponseBytes)
	s := &Session{c: c, nextID: 1}

	var msg wsMessage
	if err := s.read(ctx, &msg); err != nil || msg.Type != "auth_required" {
		s.Close()
		return nil, fmt.Errorf("Home Assistant WebSocket: no auth_required message (%v)", err)
	}
	if err := s.write(ctx, map[string]any{"type": "auth", "access_token": cfg.Token}); err != nil {
		s.Close()
		return nil, err
	}
	if err := s.read(ctx, &msg); err != nil {
		s.Close()
		return nil, err
	}
	switch msg.Type {
	case "auth_ok":
		return s, nil
	case "auth_invalid":
		s.Close()
		return nil, ErrUnauthorized
	default:
		s.Close()
		return nil, fmt.Errorf("Home Assistant WebSocket: unexpected %q after auth", msg.Type)
	}
}

// Call sends a command and decodes its result into out, when out is not nil.
func (s *Session) Call(ctx context.Context, command map[string]any, out any) error {
	id := s.nextID
	s.nextID++
	command["id"] = id
	if err := s.write(ctx, command); err != nil {
		return err
	}
	for {
		var msg wsMessage
		if err := s.read(ctx, &msg); err != nil {
			return err
		}
		if msg.ID != id || msg.Type != "result" {
			continue
		}
		if !msg.Success {
			if msg.Error != nil {
				return fmt.Errorf("Home Assistant %s: %s: %s", command["type"], msg.Error.Code, msg.Error.Message)
			}
			return fmt.Errorf("Home Assistant %s failed", command["type"])
		}
		if out == nil {
			return nil
		}
		return json.Unmarshal(msg.Result, out)
	}
}

func (s *Session) Close() {
	_ = s.c.Close(websocket.StatusNormalClosure, "")
}

func (s *Session) write(ctx context.Context, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return err
	}
	return s.c.Write(ctx, websocket.MessageText, b)
}

func (s *Session) read(ctx context.Context, v any) error {
	_, b, err := s.c.Read(ctx)
	if err != nil {
		return fmt.Errorf("Home Assistant WebSocket: %w", err)
	}
	return json.Unmarshal(b, v)
}
