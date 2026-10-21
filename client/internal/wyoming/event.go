// Package wyoming implements the Wyoming protocol over TCP: a JSON header line,
// then data_length bytes of JSON data, then payload_length bytes of payload.
package wyoming

import (
	"bufio"
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
)

// ProtocolVersion is the oldest Wyoming version of the selected services (openWakeWord).
const ProtocolVersion = "1.8.0"

const (
	TypeDescribe    = "describe"
	TypeInfo        = "info"
	TypeError       = "error"
	TypeAudioStart  = "audio-start"
	TypeAudioChunk  = "audio-chunk"
	TypeAudioStop   = "audio-stop"
	TypeDetect      = "detect"
	TypeDetection   = "detection"
	TypeNotDetected = "not-detected"
	TypeTranscribe  = "transcribe"
	TypeTranscript  = "transcript"
	TypeSynthesize  = "synthesize"
)

// The Piper info event is about 86 KB of data, so maxDataBytes leaves a large margin.
const (
	maxHeaderBytes  = 64 << 10
	maxDataBytes    = 1 << 20
	maxPayloadBytes = 4 << 20
)

var (
	ErrMalformed       = errors.New("malformed wyoming event")
	ErrUnexpectedEvent = errors.New("unexpected wyoming event")
)

// Event is one Wyoming message. Data is a JSON object or empty.
type Event struct {
	Type    string
	Data    json.RawMessage
	Payload []byte
}

func NewEvent(typ string, data any, payload []byte) (Event, error) {
	ev := Event{Type: typ, Payload: payload}
	if data != nil {
		b, err := json.Marshal(data)
		if err != nil {
			return Event{}, fmt.Errorf("encode %s data: %w", typ, err)
		}
		ev.Data = b
	}
	return ev, nil
}

// Decode unmarshals the event data into v. Empty data leaves v unchanged.
func (e Event) Decode(v any) error {
	if len(e.Data) == 0 {
		return nil
	}
	if err := json.Unmarshal(e.Data, v); err != nil {
		return fmt.Errorf("decode %s data: %w", e.Type, err)
	}
	return nil
}

// ServiceError is an "error" event sent by a service.
type ServiceError struct {
	Text string
	Code string
}

func (e *ServiceError) Error() string {
	if e.Code == "" {
		return "service error: " + e.Text
	}
	return fmt.Sprintf("service error %s: %s", e.Code, e.Text)
}

// CheckType returns nil when ev has one of the wanted types, a *ServiceError
// for an "error" event, or else an ErrUnexpectedEvent error.
func CheckType(ev Event, want ...string) error {
	if slices.Contains(want, ev.Type) {
		return nil
	}
	if ev.Type == TypeError {
		var e Error
		if err := ev.Decode(&e); err != nil {
			return err
		}
		return &ServiceError{Text: e.Text, Code: e.Code}
	}
	return fmt.Errorf("%w %q, want %s", ErrUnexpectedEvent, ev.Type, strings.Join(want, " or "))
}

type header struct {
	Type          string          `json:"type"`
	Version       string          `json:"version,omitempty"`
	Data          json.RawMessage `json:"data,omitempty"`
	DataLength    int             `json:"data_length,omitempty"`
	PayloadLength int             `json:"payload_length,omitempty"`
}

// WriteEvent writes ev with a single Write call.
func WriteEvent(w io.Writer, ev Event) error {
	data := ev.Data
	if isEmptyData(data) {
		data = nil
	}
	line, err := json.Marshal(header{
		Type:          ev.Type,
		Version:       ProtocolVersion,
		DataLength:    len(data),
		PayloadLength: len(ev.Payload),
	})
	if err != nil {
		return err
	}
	frame := make([]byte, 0, len(line)+1+len(data)+len(ev.Payload))
	frame = append(frame, line...)
	frame = append(frame, '\n')
	frame = append(frame, data...)
	frame = append(frame, ev.Payload...)
	_, err = w.Write(frame)
	return err
}

// ReadEvent reads one event. It returns io.EOF when the stream ends before an
// event, an io.ErrUnexpectedEOF error when it ends inside one, and an
// ErrMalformed error for an invalid event or one over the size limits.
func ReadEvent(r *bufio.Reader) (Event, error) {
	line, err := readLine(r)
	if err != nil {
		return Event{}, err
	}
	var h header
	if err := json.Unmarshal(line, &h); err != nil {
		return Event{}, fmt.Errorf("%w: header is not valid JSON: %v", ErrMalformed, err)
	}
	if h.Type == "" {
		return Event{}, fmt.Errorf("%w: header has no type", ErrMalformed)
	}
	if h.DataLength < 0 || h.DataLength > maxDataBytes {
		return Event{}, fmt.Errorf("%w: %s data_length %d is not from 0 to %d", ErrMalformed, h.Type, h.DataLength, maxDataBytes)
	}
	if h.PayloadLength < 0 || h.PayloadLength > maxPayloadBytes {
		return Event{}, fmt.Errorf("%w: %s payload_length %d is not from 0 to %d", ErrMalformed, h.Type, h.PayloadLength, maxPayloadBytes)
	}

	block, err := readBlock(r, h.DataLength, h.Type, "data")
	if err != nil {
		return Event{}, err
	}
	data, err := mergeData(h.Data, block)
	if err != nil {
		return Event{}, fmt.Errorf("%w: %s %v", ErrMalformed, h.Type, err)
	}
	payload, err := readBlock(r, h.PayloadLength, h.Type, "payload")
	if err != nil {
		return Event{}, err
	}
	return Event{Type: h.Type, Data: data, Payload: payload}, nil
}

func readLine(r *bufio.Reader) ([]byte, error) {
	var line []byte
	for {
		chunk, err := r.ReadSlice('\n')
		line = append(line, chunk...)
		if len(line) > maxHeaderBytes {
			return nil, fmt.Errorf("%w: header is longer than %d bytes", ErrMalformed, maxHeaderBytes)
		}
		switch {
		case err == nil:
			return line, nil
		case errors.Is(err, bufio.ErrBufferFull):
		case errors.Is(err, io.EOF) && len(line) == 0:
			return nil, io.EOF
		case errors.Is(err, io.EOF):
			return nil, fmt.Errorf("read header: %w", io.ErrUnexpectedEOF)
		default:
			return nil, err
		}
	}
}

func readBlock(r *bufio.Reader, n int, typ, part string) ([]byte, error) {
	if n == 0 {
		return nil, nil
	}
	block := make([]byte, n)
	if _, err := io.ReadFull(r, block); err != nil {
		if errors.Is(err, io.EOF) {
			err = io.ErrUnexpectedEOF
		}
		return nil, fmt.Errorf("read %s %s: %w", typ, part, err)
	}
	return block, nil
}

// mergeData joins the inline header data and the data block; the block wins,
// as in the reference implementation.
func mergeData(inline, block json.RawMessage) (json.RawMessage, error) {
	if len(inline) == 0 && len(block) == 0 {
		return nil, nil
	}
	fields := map[string]json.RawMessage{}
	for _, part := range []json.RawMessage{inline, block} {
		if len(part) == 0 {
			continue
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(part, &object); err != nil {
			return nil, errors.New("data is not a JSON object")
		}
		maps.Copy(fields, object)
	}
	return json.Marshal(fields)
}

func isEmptyData(data json.RawMessage) bool {
	switch string(bytes.TrimSpace(data)) {
	case "", "{}", "null":
		return true
	}
	return false
}
