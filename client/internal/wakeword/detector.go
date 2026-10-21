// Package wakeword streams microphone audio to openWakeWord and reports detections.
package wakeword

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"sync"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

const (
	// openWakeWord processes audio in 80 ms frames.
	chunkDuration = 80 * time.Millisecond
	// sentWindow is the number of recent chunks (5.12 s) whose send time is kept
	// to measure latency.
	sentWindow = 64

	defaultRetryMin = 500 * time.Millisecond
	defaultRetryMax = 30 * time.Second
)

// ErrUnknownModel means the service does not offer the configured model. The
// service would silently listen for its default model instead, so Run stops.
var ErrUnknownModel = errors.New("wake-word model not offered by the service")

type Detection struct {
	Model string
	// StreamOffset is the start of the detected 80 ms chunk, from the start of
	// the current service connection.
	StreamOffset time.Duration
	// Latency is the time from sending the detected chunk to receiving the
	// detection, or 0 when that chunk is no longer known.
	Latency time.Duration
	At      time.Time
}

type Config struct {
	Addr     string
	Model    string
	Cooldown time.Duration
	Timeouts wyoming.Timeouts
	Logger   *slog.Logger
}

type Detector struct {
	cfg      Config
	format   audio.Format
	retryMin time.Duration
	retryMax time.Duration

	lastDetection time.Time

	mu     sync.Mutex
	sentAt [sentWindow]sentChunk
}

type sentChunk struct {
	index int
	at    time.Time
}

type sourceError struct{ err error }

func (e *sourceError) Error() string { return "audio source: " + e.err.Error() }
func (e *sourceError) Unwrap() error { return e.err }

func New(cfg Config, format audio.Format) *Detector {
	return &Detector{cfg: cfg, format: format, retryMin: defaultRetryMin, retryMax: defaultRetryMax}
}

// Run streams audio from src, in the format given to New, and sends each
// detection to out until ctx ends.
// After a service or network failure it reconnects with a growing delay; audio
// captured meanwhile is lost or delayed within the bounds of src. Run returns
// ctx.Err(), an ErrUnknownModel error, or the error of src.
func (d *Detector) Run(ctx context.Context, src audio.Source, out chan<- Detection) error {
	delay := d.retryMin
	for {
		streamed, err := d.session(ctx, src, out)
		var srcErr *sourceError
		switch {
		case ctx.Err() != nil:
			return ctx.Err()
		case errors.Is(err, ErrUnknownModel), errors.As(err, &srcErr):
			return err
		}
		if streamed {
			delay = d.retryMin
		}
		d.cfg.Logger.Warn("wake-word service failed, retrying", "error", err, "retry_in", delay)
		select {
		case <-time.After(delay):
		case <-ctx.Done():
			return ctx.Err()
		}
		delay = min(delay*2, d.retryMax)
	}
}

// Wait runs the detector until the first detection, then stops it, so that
// the caller can read src. It returns the errors of Run.
func (d *Detector) Wait(ctx context.Context, src audio.Source) (Detection, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	out := make(chan Detection)
	done := make(chan error, 1)
	go func() { done <- d.Run(ctx, src, out) }()
	select {
	case det := <-out:
		cancel()
		<-done
		return det, nil
	case err := <-done:
		return Detection{}, err
	}
}

// session runs one connection. streamed reports whether audio streaming started.
func (d *Detector) session(ctx context.Context, src audio.Source, out chan<- Detection) (streamed bool, err error) {
	if err := d.checkModel(ctx); err != nil {
		return false, err
	}
	// No operation timeout for reads: a detection can come after hours.
	conn, err := wyoming.Dial(ctx, d.cfg.Addr, wyoming.Timeouts{Connect: d.cfg.Timeouts.Connect})
	if err != nil {
		return false, err
	}
	defer conn.Close()

	format := wyoming.AudioFormat{Rate: d.format.SampleRate, Width: audio.BytesPerSample, Channels: d.format.Channels}
	detect, err := wyoming.NewEvent(wyoming.TypeDetect, wyoming.Detect{Names: []string{d.cfg.Model}}, nil)
	if err != nil {
		return false, err
	}
	start, err := wyoming.NewEvent(wyoming.TypeAudioStart, format, nil)
	if err != nil {
		return false, err
	}
	chunk, err := wyoming.NewEvent(wyoming.TypeAudioChunk, format, nil)
	if err != nil {
		return false, err
	}
	for _, ev := range []wyoming.Event{detect, start} {
		if err := d.write(ctx, conn, ev); err != nil {
			return false, err
		}
	}
	d.resetSent()
	d.cfg.Logger.Info("listening for wake word", "model", d.cfg.Model, "addr", d.cfg.Addr)

	ctx, cancel := context.WithCancel(ctx)
	var readErr error
	readDone := make(chan struct{})
	go func() {
		defer close(readDone)
		readErr = d.readDetections(ctx, conn, out)
		cancel()
	}()
	stop := func() error {
		cancel()
		<-readDone
		return readErr
	}

	chunk.Payload = make([]byte, d.format.BytesFor(chunkDuration))
	for i := 0; ; i++ {
		if err := audio.ReadFull(ctx, src, chunk.Payload); err != nil {
			if ctx.Err() == nil {
				stop()
				return true, &sourceError{err}
			}
			return true, stop()
		}
		d.markSent(i, time.Now())
		if err := d.write(ctx, conn, chunk); err != nil {
			if ctx.Err() == nil {
				stop()
				return true, err
			}
			return true, stop()
		}
	}
}

func (d *Detector) checkModel(ctx context.Context) error {
	info, err := wyoming.Describe(ctx, d.cfg.Addr, d.cfg.Timeouts)
	if err != nil {
		return err
	}
	var names []string
	for _, program := range info.Wake {
		for _, m := range program.Models {
			if m.Name == d.cfg.Model {
				return nil
			}
			names = append(names, m.Name)
		}
	}
	return fmt.Errorf("%w: %q; available: %s", ErrUnknownModel, d.cfg.Model, strings.Join(names, ", "))
}

func (d *Detector) write(ctx context.Context, conn *wyoming.Conn, ev wyoming.Event) error {
	ctx, cancel := context.WithTimeout(ctx, d.cfg.Timeouts.Operation)
	defer cancel()
	return conn.Write(ctx, ev)
}

func (d *Detector) readDetections(ctx context.Context, conn *wyoming.Conn, out chan<- Detection) error {
	for {
		ev, err := conn.Read(ctx)
		if err != nil {
			return err
		}
		if err := wyoming.CheckType(ev, wyoming.TypeDetection); err != nil {
			return err
		}
		var det wyoming.Detection
		if err := ev.Decode(&det); err != nil {
			return err
		}
		if err := d.emit(ctx, det, out); err != nil {
			return err
		}
	}
}

// emit sends det to out unless it comes within the cooldown of the last one.
// The cooldown also covers a reconnect, where the server refractory time restarts.
func (d *Detector) emit(ctx context.Context, det wyoming.Detection, out chan<- Detection) error {
	now := time.Now()
	if !d.lastDetection.IsZero() && now.Sub(d.lastDetection) < d.cfg.Cooldown {
		d.cfg.Logger.Debug("detection ignored in cooldown", "model", det.Name)
		return nil
	}
	d.lastDetection = now
	offset := time.Duration(det.Timestamp) * time.Millisecond
	select {
	case out <- Detection{Model: det.Name, StreamOffset: offset, Latency: d.latency(offset, now), At: now}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *Detector) resetSent() {
	d.mu.Lock()
	d.sentAt = [sentWindow]sentChunk{}
	d.mu.Unlock()
}

func (d *Detector) markSent(index int, at time.Time) {
	d.mu.Lock()
	d.sentAt[index%sentWindow] = sentChunk{index: index, at: at}
	d.mu.Unlock()
}

func (d *Detector) latency(offset time.Duration, received time.Time) time.Duration {
	index := int(offset / chunkDuration)
	d.mu.Lock()
	sent := d.sentAt[index%sentWindow]
	d.mu.Unlock()
	if sent.at.IsZero() || sent.index != index {
		return 0
	}
	return received.Sub(sent.at)
}
