// Package llm turns a transcribed Spanish device command into commands with a
// local llama.cpp server. The answer is constrained to turn_on/turn_off and the
// allowlisted device IDs, so the model cannot name another action, service or
// entity.
package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
	"unicode"

	"jarvis/client/internal/commands"
)

const (
	maxResponseBytes = 1 << 20
	maxCommands      = 4
	maxTokens        = 128
)

// Config points at the OpenAI-compatible API of llama.cpp.
type Config struct {
	// URL is the base URL, for example http://127.0.0.1:8080.
	URL     string
	Timeout time.Duration
	// Room is where the assistant is, for example "Salón". It lets the model
	// resolve "aquí" to the devices of that room. Empty when unknown.
	Room string
}

// Client asks a llama.cpp server for commands. It is safe for concurrent use.
type Client struct {
	url  string
	room string
	http *http.Client
}

func New(cfg Config) *Client {
	return &Client{
		url:  strings.TrimRight(cfg.URL, "/"),
		room: strings.TrimSpace(cfg.Room),
		http: &http.Client{Timeout: cfg.Timeout},
	}
}

// Interpret maps text to commands, using only the devices in the allowlist.
// The text may be an explicit command or a state that implies one. It does not
// call the server without devices. An empty result means that the model found
// no command.
func (c *Client) Interpret(ctx context.Context, text string, devices []commands.Device) ([]commands.Command, error) {
	if len(devices) == 0 {
		return []commands.Command{}, nil
	}
	body, err := json.Marshal(request{
		Model:       "local",
		Messages:    []message{{Role: "system", Content: prompt(devices, c.room)}, {Role: "user", Content: text}},
		Temperature: 0,
		TopK:        1,
		MaxTokens:   maxTokens,
		CachePrompt: true,
		ResponseFormat: responseFormat{
			Type:       "json_schema",
			JSONSchema: jsonSchema{Name: "commands", Strict: true, Schema: schema(devices)},
		},
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.url+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("llama.cpp: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("llama.cpp: HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(b)))
	}
	var completion completion
	if err := json.NewDecoder(io.LimitReader(resp.Body, maxResponseBytes)).Decode(&completion); err != nil {
		return nil, fmt.Errorf("llama.cpp: %w", err)
	}
	if len(completion.Choices) == 0 {
		return nil, errors.New("llama.cpp: no choices")
	}
	content := strings.TrimSpace(completion.Choices[0].Message.Content)
	if content == "" {
		return nil, errors.New("llama.cpp: empty answer")
	}
	var answer answer
	if err := json.Unmarshal([]byte(content), &answer); err != nil {
		return nil, fmt.Errorf("llama.cpp: invalid JSON: %w", err)
	}
	return commandsFrom(answer, devices)
}

type message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type request struct {
	Model          string         `json:"model"`
	Messages       []message      `json:"messages"`
	Temperature    float64        `json:"temperature"`
	TopK           int            `json:"top_k"`
	MaxTokens      int            `json:"max_tokens"`
	CachePrompt    bool           `json:"cache_prompt"`
	ResponseFormat responseFormat `json:"response_format"`
}

type responseFormat struct {
	Type       string     `json:"type"`
	JSONSchema jsonSchema `json:"json_schema"`
}

type jsonSchema struct {
	Name   string `json:"name"`
	Strict bool   `json:"strict"`
	Schema any    `json:"schema"`
}

type completion struct {
	Choices []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	} `json:"choices"`
}

type answer struct {
	Commands []struct {
		Action string `json:"action"`
		Device string `json:"device"`
	} `json:"commands"`
}

// schema allows only turn_on/turn_off and the allowlisted device IDs. Interpret
// does not call the server without devices.
func schema(devices []commands.Device) any {
	ids := make([]string, len(devices))
	for i, d := range devices {
		ids[i] = d.ID
	}
	item := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{"type": "string", "enum": []string{"turn_on", "turn_off"}},
			"device": map[string]any{"type": "string", "enum": ids},
		},
		"required":             []string{"action"},
		"additionalProperties": false,
	}
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"commands": map[string]any{
				"type":     "array",
				"maxItems": maxCommands,
				"items":    item,
			},
		},
		"required":             []string{"commands"},
		"additionalProperties": false,
	}
}

// prompt is a few-shot interpreter prompt. It is stable for the same allowlist
// and room, so llama.cpp can reuse its cached prefix on the next request.
func prompt(devices []commands.Device, room string) string {
	var b strings.Builder
	b.WriteString("You are a voice assistant in a house. Convert the user's Spanish sentence into an action on a device.\n")
	if room != "" {
		fmt.Fprintf(&b, "The assistant is in the room %q. The words \"aquí\", \"aqui\" or \"en esta habitación\" refer to it.\n", room)
	}
	b.WriteString(`
Actions:
- turn_on: turn a device on.
- turn_off: turn a device off.

Devices:
`)
	for _, d := range devices {
		fmt.Fprintf(&b, "- %s = %s\n", d.ID, d.Name)
	}
	b.WriteString(`
Rules:
- Use only the IDs above, with turn_on or turn_off.
- Return several actions in the order requested.
- If the sentence asks to turn a device on or off, use that device.
- If the user describes a problem or a feeling instead of a command, choose the action that fixes it: too much light or brightness -> turn_off a light; darkness or cannot see -> turn_on a light; cold -> turn_on the air conditioner; hot -> turn_off the air conditioner.
- A sentence with "no" can still describe a state: "no veo nada" and "no hay luz" mean turn_on a light, not turn_off.
`)
	if room != "" {
		fmt.Fprintf(&b, "- If no other room is named, a problem such as \"hay demasiada luz\" refers to the current room, %q.\n", room)
	}
	b.WriteString(`- Return {"commands": []} if the sentence is not about controlling a device, if it is a question, or if it is not possible.
- One action for a problem or a feeling. Do not invent actions or devices.
- Naming a device without asking for anything is not a command.
- Ignore politeness and filler words.

Examples:
`)
	// One example per device teaches the exact name-to-ID mapping. A 0.5B
	// model needs this when several names share words ("la luz de...").
	for _, d := range devices {
		fmt.Fprintf(&b, "Usuario: enciende %s\nAsistente: %s\n", d.Name, example(action{Kind: "turn_on", Device: d.ID}))
	}
	if len(devices) > 0 {
		d0 := devices[0]
		fmt.Fprintf(&b, "Usuario: apaga %s por favor\nAsistente: %s\n", d0.Name, example(action{Kind: "turn_off", Device: d0.ID}))
	}
	if len(devices) > 1 {
		d0, d1 := devices[0], devices[1]
		fmt.Fprintf(&b, "Usuario: prende %s y apaga %s\nAsistente: %s\n", d0.Name, d1.Name, example(action{Kind: "turn_on", Device: d0.ID}, action{Kind: "turn_off", Device: d1.ID}))
	}
	// A state with no device name needs the room, so these examples are only
	// useful when the assistant knows where it is.
	if d, ok := deviceInRoom(devices, room); ok {
		fmt.Fprintf(&b, "Usuario: hay demasiada luz aquí\nAsistente: %s\n", example(action{Kind: "turn_off", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: está muy oscuro\nAsistente: %s\n", example(action{Kind: "turn_on", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: no veo nada\nAsistente: %s\n", example(action{Kind: "turn_on", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: me sobra luz\nAsistente: %s\n", example(action{Kind: "turn_off", Device: d.ID}))
	}
	if d, ok := airConditioner(devices); ok {
		fmt.Fprintf(&b, "Usuario: hace frío\nAsistente: %s\n", example(action{Kind: "turn_on", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: tengo calor\nAsistente: %s\n", example(action{Kind: "turn_off", Device: d.ID}))
	}
	b.WriteString(`Usuario: pon la tele
Asistente: {"commands": []}
Usuario: ¿quieres apagar la luz?
Asistente: {"commands": []}
Usuario: ¿qué hora es?
Asistente: {"commands": []}
Usuario: cuéntame un chiste
Asistente: {"commands": []}
`)
	// These last examples are close to the user's sentence, which helps a
	// 0.5B model treat "no veo nada" as darkness and not as a refusal.
	if d, ok := deviceInRoom(devices, room); ok {
		fmt.Fprintf(&b, "Usuario: no veo nada\nAsistente: %s\n", example(action{Kind: "turn_on", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: no hay luz\nAsistente: %s\n", example(action{Kind: "turn_on", Device: d.ID}))
		fmt.Fprintf(&b, "Usuario: hay demasiada luz\nAsistente: %s\n", example(action{Kind: "turn_off", Device: d.ID}))
	}
	return b.String()
}

var plainLetters = strings.NewReplacer("á", "a", "é", "e", "í", "i", "ó", "o", "ú", "u", "ü", "u", "ñ", "n")

// deviceInRoom returns the light of the current room, for the examples that
// turn a state such as "hay demasiada luz" into an action. The device name
// usually contains the room ("la luz del salón").
func deviceInRoom(devices []commands.Device, room string) (commands.Device, bool) {
	want := normalize(room)
	if want == "" {
		return commands.Device{}, false
	}
	for _, d := range devices {
		name := normalize(d.Name)
		isLight := strings.HasPrefix(d.ID, "light.") || strings.Contains(name, "luz") || strings.Contains(name, "lampara")
		if isLight && strings.Contains(name, want) {
			return d, true
		}
	}
	return commands.Device{}, false
}

// airConditioner returns the device for the temperature examples.
func airConditioner(devices []commands.Device) (commands.Device, bool) {
	for _, d := range devices {
		name := normalize(d.Name)
		if strings.Contains(name, "aire") || strings.Contains(name, "calefacc") || strings.Contains(name, "clima") {
			return d, true
		}
	}
	return commands.Device{}, false
}

func normalize(text string) string {
	text = plainLetters.Replace(strings.ToLower(text))
	words := strings.FieldsFunc(text, func(r rune) bool {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	})
	return strings.Join(words, " ")
}

type action struct {
	Kind   string
	Device string
}

// example formats the few-shot answer with the spaces that the model expects.
// The compact JSON encoding of the same object makes the model return an empty
// list; the exact format matters for a 0.5B model.
func example(actions ...action) string {
	items := make([]string, len(actions))
	for i, a := range actions {
		if a.Device == "" {
			items[i] = fmt.Sprintf(`{"action": %q}`, a.Kind)
			continue
		}
		items[i] = fmt.Sprintf(`{"action": %q, "device": %q}`, a.Kind, a.Device)
	}
	return `{"commands": [` + strings.Join(items, ", ") + `]}`
}

// commandsFrom checks the answer again, in case the server ignored the schema.
func commandsFrom(a answer, devices []commands.Device) ([]commands.Command, error) {
	if len(a.Commands) == 0 {
		return []commands.Command{}, nil
	}
	if len(a.Commands) > maxCommands {
		return nil, fmt.Errorf("llama.cpp: %d commands, at most %d", len(a.Commands), maxCommands)
	}
	byID := make(map[string]commands.Device, len(devices))
	for _, d := range devices {
		byID[d.ID] = d
	}
	out := make([]commands.Command, 0, len(a.Commands))
	for _, item := range a.Commands {
		switch item.Action {
		case "turn_on", "turn_off":
			device, ok := byID[item.Device]
			if !ok {
				return nil, fmt.Errorf("llama.cpp: unknown device %q", item.Device)
			}
			kind := commands.KindTurnOn
			if item.Action == "turn_off" {
				kind = commands.KindTurnOff
			}
			out = append(out, commands.Command{Kind: kind, Device: device})
		default:
			return nil, fmt.Errorf("llama.cpp: unknown action %q", item.Action)
		}
	}
	return out, nil
}
