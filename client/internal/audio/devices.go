// Package audio wraps the native audio library (malgo). The rest of the client
// uses only the types of this package.
package audio

import (
	"fmt"
	"strings"

	"github.com/gen2brain/malgo"
)

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
}

func Open() (*Backend, error) {
	ctx, err := malgo.InitContext(nil, malgo.ContextConfig{}, nil)
	if err != nil {
		return nil, fmt.Errorf("open audio backend: %w", err)
	}
	return &Backend{ctx: ctx}, nil
}

// Close releases the backend. Close all streams first.
func (b *Backend) Close() {
	_ = b.ctx.Uninit()
	b.ctx.Free()
}

func (b *Backend) Devices() (Devices, error) {
	inputs, err := b.devices(malgo.Capture)
	if err != nil {
		return Devices{}, fmt.Errorf("list input devices: %w", err)
	}
	outputs, err := b.devices(malgo.Playback)
	if err != nil {
		return Devices{}, fmt.Errorf("list output devices: %w", err)
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
func findDevice(devices []Device, label string) (*Device, error) {
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
	return nil, fmt.Errorf("device %q not found; available: %s", label, strings.Join(quoted, ", "))
}
