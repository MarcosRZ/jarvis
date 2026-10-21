package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"runtime"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/config"
)

const (
	captureBuffer  = 2 * time.Second
	playbackBuffer = time.Second
	levelInterval  = time.Second
	bytesPerMiB    = 1 << 20

	// macOS gives zero samples, not an error, to an app with no microphone permission.
	silenceWarning = "all captured samples are zero; on macOS, give the terminal app microphone access in System Settings > Privacy & Security > Microphone"
)

// openMicrophone opens the backend and starts capture in the speech format.
// Close the capture, then the backend.
func openMicrophone(device string, logger *slog.Logger) (*audio.Backend, *audio.Capture, error) {
	backend, err := audio.Open()
	if err != nil {
		return nil, nil, err
	}
	capture, err := backend.OpenCapture(device, audio.SpeechFormat, captureBuffer)
	if err != nil {
		backend.Close()
		return nil, nil, err
	}
	logStreamFormat(logger, "capture started", capture.Format(), capture.NativeFormat())
	return backend, capture, nil
}

func micTest(ctx context.Context, cfg config.Config, duration time.Duration, stdout io.Writer, logger *slog.Logger) int {
	backend, capture, err := openMicrophone(cfg.InputDevice, logger)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()
	defer capture.Close()

	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	heapStart := heapAlloc()
	started := time.Now()
	format := capture.Format()
	window := format.BytesFor(levelInterval)
	buf := make([]byte, window)
	total, inWindow, peak, overallPeak := 0, 0, math.Inf(-1), math.Inf(-1)
	for {
		n, err := capture.Read(ctx, buf)
		if err != nil {
			if !isStop(err) {
				logger.Error("capture failed", "error", err)
				return exitError
			}
			break
		}
		total += n
		inWindow += n
		peak = max(peak, audio.PeakDBFS(buf[:n]))
		if inWindow >= window {
			fmt.Fprintf(stdout, "%6s  peak %s\n", format.Duration(total).Truncate(time.Second), levelBar(peak))
			overallPeak = max(overallPeak, peak)
			inWindow, peak = 0, math.Inf(-1)
		}
	}
	if overallPeak = max(overallPeak, peak); total > 0 && math.IsInf(overallPeak, -1) {
		logger.Warn(silenceWarning)
	}

	runtime.GC()
	fmt.Fprintf(stdout, "captured %v of audio in %v, dropped %d bytes, heap %.1f MiB at start and %.1f MiB at end\n",
		format.Duration(total).Round(time.Millisecond), time.Since(started).Round(time.Millisecond),
		capture.Dropped(), float64(heapStart)/bytesPerMiB, float64(heapAlloc())/bytesPerMiB)
	return exitOK
}

func loopback(ctx context.Context, cfg config.Config, duration time.Duration, logger *slog.Logger) int {
	backend, err := audio.Open()
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer backend.Close()

	recording, err := record(ctx, backend, cfg.InputDevice, duration, logger)
	if err != nil {
		logger.Error("recording failed", "error", err)
		return exitError
	}
	if ctx.Err() != nil {
		return exitOK
	}

	playback, err := backend.OpenPlayback(cfg.OutputDevice, audio.SpeechFormat, playbackBuffer)
	if err != nil {
		logger.Error("audio", "error", err)
		return exitError
	}
	defer playback.Close()
	logStreamFormat(logger, "playing", playback.Format(), playback.NativeFormat())
	err = playback.Write(ctx, recording)
	if err == nil {
		err = playback.Drain(ctx)
	}
	if err != nil && !isStop(err) {
		logger.Error("playback failed", "error", err)
		return exitError
	}
	logger.Info("done")
	return exitOK
}

// record captures until duration has passed or ctx ends.
func record(ctx context.Context, backend *audio.Backend, device string, duration time.Duration, logger *slog.Logger) ([]byte, error) {
	capture, err := backend.OpenCapture(device, audio.SpeechFormat, captureBuffer)
	if err != nil {
		return nil, err
	}
	defer capture.Close()
	logStreamFormat(logger, "recording", capture.Format(), capture.NativeFormat())

	ctx, cancel := context.WithTimeout(ctx, duration)
	defer cancel()
	recording := make([]byte, capture.Format().BytesFor(duration))
	total := 0
	for total < len(recording) {
		n, err := capture.Read(ctx, recording[total:])
		if isStop(err) {
			break
		}
		if err != nil {
			return nil, err
		}
		total += n
	}
	peak := audio.PeakDBFS(recording[:total])
	logger.Info("recorded", "audio", capture.Format().Duration(total), "peak_dbfs", peak, "dropped_bytes", capture.Dropped())
	if total > 0 && math.IsInf(peak, -1) {
		logger.Warn(silenceWarning)
	}
	return recording[:total], nil
}

func logStreamFormat(logger *slog.Logger, msg string, format, native audio.Format) {
	logger.Info(msg, "rate", format.SampleRate, "channels", format.Channels,
		"device_rate", native.SampleRate, "device_channels", native.Channels, "converted", format != native)
}

func isStop(err error) bool {
	return errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled)
}

func heapAlloc() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.HeapAlloc
}

const (
	meterFloorDB = -60
	meterWidth   = 30
)

func levelBar(db float64) string {
	filled := 0
	if db > meterFloorDB {
		filled = int((db - meterFloorDB) / -meterFloorDB * meterWidth)
	}
	bar := make([]byte, meterWidth)
	for i := range bar {
		bar[i] = '.'
		if i < filled {
			bar[i] = '#'
		}
	}
	return fmt.Sprintf("%7.1f dBFS [%s]", db, bar)
}
