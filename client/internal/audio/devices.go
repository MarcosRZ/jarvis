// Package audio wraps the native audio library (malgo). The rest of the client
// uses only the types of this package.
package audio

import (
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/gen2brain/malgo"
)

// keptMessages is how many recent miniaudio messages a failed open reports.
const keptMessages = 5

// Device is an audio device. Label is the device name, with a number suffix
// when more devices of the same kind have the same name.
type Device struct {
	Label   string
	Default bool
	id      malgo.DeviceID
}

type Devices struct {
	Inputs  []Device
	Outputs []Device
}

// Backend is an open connection to the system audio API.
type Backend struct {
	ctx *malgo.AllocatedContext
	log *messages
}

// messages keeps the last miniaudio messages, which explain its errors: a
// malgo error alone is often only "unknown error".
type messages struct {
	mu     sync.Mutex
	recent []string
}

func (m *messages) add(msg string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.recent = append(m.recent, msg)
	if len(m.recent) > keptMessages {
		m.recent = m.recent[1:]
	}
}

// forget drops the messages of an operation that worked.
func (m *messages) forget() {
	m.mu.Lock()
	m.recent = nil
	m.mu.Unlock()
}

// explain adds the recent messages to err and forgets them.
func (m *messages) explain(err error) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.recent) == 0 {
		return err
	}
	err = fmt.Errorf("%w (miniaudio: %s)", err, strings.Join(m.recent, "; "))
	m.recent = nil
	return err
}

// Open connects to the system audio API. logger, if not nil, gets each
// miniaudio message at debug level.
func Open(logger *slog.Logger) (*Backend, error) {
	log := &messages{}
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, func(msg string) {
		msg = strings.TrimSpace(msg)
		log.add(msg)
		if logger != nil {
			logger.Debug("miniaudio", "message", msg)
		}
	})
	if err != nil {
		return nil, log.explain(fmt.Errorf("open audio backend: %w", err))
	}
	return &Backend{ctx: ctx, log: log}, nil
}

// Close releases the backend. Close all streams first.
func (b *Backend) Close() {
	_ = b.ctx.Uninit()
	b.ctx.Free()
}

func (b *Backend) Devices() (Devices, error) {
	inputs, err := b.devices(malgo.Capture)
	if err != nil {
		return Devices{}, b.log.explain(fmt.Errorf("list input devices: %w", err))
	}
	outputs, err := b.devices(malgo.Playback)
	if err != nil {
		return Devices{}, b.log.explain(fmt.Errorf("list output devices: %w", err))
	}
	return Devices{Inputs: inputs, Outputs: outputs}, nil
}

func (b *Backend) devices(kind malgo.DeviceType) ([]Device, error) {
	infos, err := b.ctx.Devices(kind)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(infos))
	for i := range infos {
		names[i] = infos[i].Name()
	}
	result := make([]Device, len(infos))
	for i, label := range labelNames(names) {
		result[i] = Device{Label: label, Default: infos[i].IsDefault != 0, id: infos[i].ID}
	}
	return result, nil
}

func labelNames(names []string) []string {
	seen := make(map[string]int, len(names))
	labels := make([]string, len(names))
	for i, name := range names {
		seen[name]++
		labels[i] = name
		if n := seen[name]; n > 1 {
			labels[i] = fmt.Sprintf("%s #%d", name, n)
		}
	}
	return labels
}

// findDevice returns the device with label, or nil for the system default when label is empty.
// findDevice returns nil for the system default (label ""). kind is "input"
// or "output", for the error.
func findDevice(devices []Device, label, kind string) (*Device, error) {
	if len(devices) == 0 {
		return nil, fmt.Errorf("no %s device found: the system lists none; connect one and check the sound settings", kind)
	}
	if label == "" {
		return nil, nil
	}
	quoted := make([]string, len(devices))
	for i := range devices {
		if devices[i].Label == label {
			return &devices[i], nil
		}
		quoted[i] = fmt.Sprintf("%q", devices[i].Label)
	}
	return nil, fmt.Errorf("%s device %q not found; available: %s", kind, label, strings.Join(quoted, ", "))
}
