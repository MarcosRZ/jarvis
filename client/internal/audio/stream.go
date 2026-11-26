package audio

import (
	"context"
	"errors"
	"fmt"
	"runtime"
	"sync/atomic"
	"time"

	"github.com/gen2brain/malgo"
)

var (
	ErrDeviceStopped = errors.New("audio device stopped unexpectedly; check that it is still connected")
	ErrClosed        = errors.New("audio stream closed")
)

// drainTail is the time the device needs to play the samples that it already
// took from the ring.
const drainTail = 200 * time.Millisecond

// tapBuffer is how much captured audio the diagnostic tail keeps. It is a
// second reader, so a tail never takes audio from the wake word or a command.
const tapBuffer = 4 * time.Second

type stream struct {
	device  *malgo.Device
	ring    *Ring
	format  Format
	native  Format
	pinner  runtime.Pinner
	closing atomic.Bool
}

// Format is the format that the stream gives or takes.
func (s *stream) Format() Format { return s.format }

// NativeFormat is the device format. When it differs from Format, miniaudio
// converts between the two.
func (s *stream) NativeFormat() Format { return s.native }

// Close stops the device and releases it. Blocked calls return ErrClosed.
func (s *stream) Close() {
	s.closing.Store(true)
	s.device.Uninit()
	s.pinner.Unpin()
	s.ring.Close(ErrClosed)
}

func (s *stream) onStop() {
	if !s.closing.Load() {
		s.ring.Close(ErrDeviceStopped)
	}
}

// Capture gives microphone audio. The audio callback only copies samples into
// a bounded ring; when the reader is too slow the oldest samples are dropped.
// A second ring keeps the most recent audio for diagnostic tails.
type Capture struct {
	stream
	dropped atomic.Int64
	tap     *Ring
}

// OpenCapture opens the input device with label (empty for the system default)
// and starts it. buffer is the maximum audio held for the reader.
func (b *Backend) OpenCapture(label string, format Format, buffer time.Duration) (*Capture, error) {
	devices, err := b.Devices()
	if err != nil {
		return nil, err
	}
	device, err := findDevice(devices.Inputs, label, "input")
	if err != nil {
		return nil, err
	}
	c := &Capture{
		stream: stream{ring: NewRing(format.BytesFor(buffer), format.FrameSize()), format: format},
		tap:    NewRing(format.BytesFor(tapBuffer), format.FrameSize()),
	}
	config := malgo.DefaultDeviceConfig(malgo.Capture)
	config.Capture.Format = malgo.FormatS16
	config.Capture.Channels = uint32(format.Channels)
	config.SampleRate = uint32(format.SampleRate)
	if device != nil {
		c.pinner.Pin(&device.id)
		config.Capture.DeviceID = device.id.Pointer()
	}
	data := func(_, input []byte, _ uint32) {
		c.dropped.Add(int64(c.ring.Push(input)))
		c.tap.Push(input)
	}
	if err := c.start(b, config, data, deviceName(label)); err != nil {
		return nil, err
	}
	c.native = Format{SampleRate: int(c.device.CaptureInternalSampleRate()), Channels: int(c.device.CaptureInternalChannels())}
	return c, nil
}

// Read waits for audio and copies up to len(p) bytes, in whole frames, into p.
func (c *Capture) Read(ctx context.Context, p []byte) (int, error) {
	return c.ring.Read(ctx, p)
}

// Discard drops the audio captured but not read yet, for example the
// assistant's own voice captured while it spoke.
func (c *Capture) Discard() { c.ring.Reset() }

// Tail returns the next d of captured audio, starting now, from a second ring
// that does not consume the main capture stream. It is used for diagnostic
// recordings after a reply.
func (c *Capture) Tail(ctx context.Context, d time.Duration) ([]byte, error) {
	c.tap.Reset()
	pcm := make([]byte, c.format.BytesFor(d))
	if err := ReadFull(ctx, c.tap, pcm); err != nil {
		return nil, err
	}
	return pcm, nil
}

// Last returns the newest d of captured audio without consuming it. It is used
// to save the audio that led to a wake-word detection.
func (c *Capture) Last(d time.Duration) []byte {
	return c.tap.Snapshot(c.format.BytesFor(d))
}

// Close stops the device and closes both capture rings.
func (c *Capture) Close() {
	c.stream.Close()
	c.tap.Close(ErrClosed)
}

// Dropped returns the number of bytes lost because the reader was too slow.
func (c *Capture) Dropped() int64 { return c.dropped.Load() }

// Playback plays audio. The audio callback only copies samples from a bounded
// ring and plays silence when the ring is empty.
type Playback struct {
	stream
}

// OpenPlayback opens the output device with label (empty for the system
// default) and starts it. buffer is the maximum audio queued for the device.
func (b *Backend) OpenPlayback(label string, format Format, buffer time.Duration) (*Playback, error) {
	devices, err := b.Devices()
	if err != nil {
		return nil, err
	}
	device, err := findDevice(devices.Outputs, label, "output")
	if err != nil {
		return nil, err
	}
	p := &Playback{stream: stream{ring: NewRing(format.BytesFor(buffer), format.FrameSize()), format: format}}
	config := malgo.DefaultDeviceConfig(malgo.Playback)
	config.Playback.Format = malgo.FormatS16
	config.Playback.Channels = uint32(format.Channels)
	config.SampleRate = uint32(format.SampleRate)
	if device != nil {
		p.pinner.Pin(&device.id)
		config.Playback.DeviceID = device.id.Pointer()
	}
	data := func(output, _ []byte, _ uint32) {
		n := p.ring.Pop(output)
		clear(output[n:])
	}
	if err := p.start(b, config, data, deviceName(label)); err != nil {
		return nil, err
	}
	p.native = Format{SampleRate: int(p.device.PlaybackInternalSampleRate()), Channels: int(p.device.PlaybackInternalChannels())}
	return p, nil
}

// Write queues pcm for playback and waits while the queue is full.
func (p *Playback) Write(ctx context.Context, pcm []byte) error {
	return p.ring.Write(ctx, pcm)
}

// Drain waits until all queued audio has been played.
func (p *Playback) Drain(ctx context.Context) error {
	if err := p.ring.WaitEmpty(ctx); err != nil {
		return err
	}
	select {
	case <-time.After(drainTail):
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (s *stream) start(b *Backend, config malgo.DeviceConfig, data malgo.DataProc, name string) error {
	device, err := malgo.InitDevice(b.ctx.Context, config, malgo.DeviceCallbacks{Data: data, Stop: s.onStop})
	if err != nil {
		s.pinner.Unpin()
		return b.log.explain(fmt.Errorf("open %s: %w", name, err))
	}
	s.device = device
	if err := device.Start(); err != nil {
		s.Close()
		return b.log.explain(fmt.Errorf("start %s: %w", name, err))
	}
	b.log.forget()
	return nil
}

func deviceName(label string) string {
	if label == "" {
		return "default audio device"
	}
	return fmt.Sprintf("audio device %q", label)
}
