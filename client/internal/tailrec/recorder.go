// Package tailrec saves short diagnostic recordings of the microphone after a
// reply. Only the most recent files are kept, so the recordings cannot fill the
// disk.
package tailrec

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync/atomic"
	"time"

	"jarvis/client/internal/audio"
)

const filePrefix = "tail-"

// Source returns d of microphone audio, starting now.
type Source interface {
	Tail(ctx context.Context, d time.Duration) ([]byte, error)
}

// Recorder writes one WAV after a reply and deletes the oldest files above
// keep. Start never blocks the caller.
type Recorder struct {
	dir      string
	keep     int
	duration time.Duration
	format   audio.Format
	source   Source
	now      func() time.Time
	logger   *slog.Logger
	running  atomic.Bool
}

func New(dir string, keep int, duration time.Duration, format audio.Format, source Source, logger *slog.Logger) *Recorder {
	return &Recorder{
		dir:      dir,
		keep:     keep,
		duration: duration,
		format:   format,
		source:   source,
		now:      time.Now,
		logger:   logger,
	}
}

// Start records one tail in the background. A request that arrives while a
// tail is still being recorded is skipped, so two tails never share the same
// capture.
func (r *Recorder) Start(ctx context.Context) {
	if !r.running.CompareAndSwap(false, true) {
		r.logger.Debug("tail recording skipped")
		return
	}
	go func() {
		defer r.running.Store(false)
		r.record(ctx)
	}()
}

func (r *Recorder) record(ctx context.Context) {
	pcm, err := r.source.Tail(ctx, r.duration)
	if err != nil {
		if ctx.Err() == nil {
			r.logger.Warn("tail recording failed", "error", err)
		}
		return
	}
	path, err := r.save(pcm)
	if err != nil {
		r.logger.Warn("tail recording failed", "error", err)
		return
	}
	r.logger.Info("tail saved", "file", path, "audio", r.format.Duration(len(pcm)).Round(10*time.Millisecond))
}

func (r *Recorder) save(pcm []byte) (string, error) {
	if err := os.MkdirAll(r.dir, 0o700); err != nil {
		return "", fmt.Errorf("create tail directory: %w", err)
	}
	name := filePrefix + r.now().Format("20060102-150405.000000000") + ".wav"
	path := filepath.Join(r.dir, name)
	if err := writeWAV(path, r.format, pcm); err != nil {
		return "", fmt.Errorf("write tail: %w", err)
	}
	if err := r.prune(); err != nil {
		r.logger.Warn("tail cleanup failed", "error", err)
	}
	return path, nil
}

// prune keeps the newest keep files, by name, which starts with the timestamp.
func (r *Recorder) prune() error {
	entries, err := os.ReadDir(r.dir)
	if err != nil {
		return err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		if !entry.IsDir() && strings.HasPrefix(name, filePrefix) && strings.HasSuffix(name, ".wav") {
			names = append(names, name)
		}
	}
	if len(names) <= r.keep {
		return nil
	}
	sort.Strings(names)
	var errs []error
	for _, name := range names[:len(names)-r.keep] {
		if err := os.Remove(filepath.Join(r.dir, name)); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}

// writeWAV writes 16-bit PCM in a RIFF/WAVE container. The file is private,
// because it can contain nearby speech.
func writeWAV(path string, format audio.Format, pcm []byte) (err error) {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := file.Close(); err == nil {
			err = closeErr
		}
	}()
	if _, err := file.Write(wavHeader(format, len(pcm))); err != nil {
		return err
	}
	_, err = file.Write(pcm)
	return err
}

func wavHeader(format audio.Format, pcmLen int) []byte {
	blockAlign := format.Channels * audio.BytesPerSample
	header := make([]byte, 44)
	copy(header[0:], "RIFF")
	binary.LittleEndian.PutUint32(header[4:], uint32(36+pcmLen))
	copy(header[8:], "WAVE")
	copy(header[12:], "fmt ")
	binary.LittleEndian.PutUint32(header[16:], 16)
	binary.LittleEndian.PutUint16(header[20:], 1) // PCM
	binary.LittleEndian.PutUint16(header[22:], uint16(format.Channels))
	binary.LittleEndian.PutUint32(header[24:], uint32(format.SampleRate))
	binary.LittleEndian.PutUint32(header[28:], uint32(format.SampleRate*blockAlign))
	binary.LittleEndian.PutUint16(header[32:], uint16(blockAlign))
	binary.LittleEndian.PutUint16(header[34:], 16)
	copy(header[36:], "data")
	binary.LittleEndian.PutUint32(header[40:], uint32(pcmLen))
	return header
}
