// Package stt sends recorded speech to a Wyoming speech-to-text service (Whisper).
package stt

import (
	"context"
	"fmt"
	"strings"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

const chunkDuration = 100 * time.Millisecond

type Config struct {
	Addr     string
	Language string
	Timeouts wyoming.Timeouts
}

type Client struct {
	cfg Config
}

// Result is a transcription. Text is empty when the service recognized nothing.
type Result struct {
	Text string
	// Processing is the time from sending audio-stop to receiving the transcript.
	Processing time.Duration
	// Total is the time from connecting to receiving the transcript.
	Total time.Duration
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg}
}

// Transcribe sends pcm in format and returns the text. Whisper transcribes only
// after audio-stop, so the client sends the whole recording at once.
func (c *Client) Transcribe(ctx context.Context, format audio.Format, pcm []byte) (Result, error) {
	start := time.Now()
	conn, err := wyoming.Dial(ctx, c.cfg.Addr, c.cfg.Timeouts)
	if err != nil {
		return Result{}, err
	}
	defer conn.Close()

	audioFormat := wyoming.AudioFormat{Rate: format.SampleRate, Width: audio.BytesPerSample, Channels: format.Channels}
	events := []struct {
		typ  string
		data any
	}{
		{wyoming.TypeTranscribe, wyoming.Transcribe{Language: c.cfg.Language}},
		{wyoming.TypeAudioStart, audioFormat},
	}
	for _, e := range events {
		if err := c.write(ctx, conn, e.typ, e.data, nil); err != nil {
			return Result{}, err
		}
	}
	step := format.BytesFor(chunkDuration)
	for i := 0; i < len(pcm); i += step {
		if err := c.write(ctx, conn, wyoming.TypeAudioChunk, audioFormat, pcm[i:min(i+step, len(pcm))]); err != nil {
			return Result{}, err
		}
	}
	if err := c.write(ctx, conn, wyoming.TypeAudioStop, nil, nil); err != nil {
		return Result{}, err
	}
	stopped := time.Now()

	ev, err := conn.Read(ctx)
	if err != nil {
		return Result{}, err
	}
	if err := wyoming.CheckType(ev, wyoming.TypeTranscript); err != nil {
		return Result{}, fmt.Errorf("transcribe at %s: %w", c.cfg.Addr, err)
	}
	var transcript wyoming.Transcript
	if err := ev.Decode(&transcript); err != nil {
		return Result{}, err
	}
	now := time.Now()
	return Result{Text: strings.TrimSpace(transcript.Text), Processing: now.Sub(stopped), Total: now.Sub(start)}, nil
}

func (c *Client) write(ctx context.Context, conn *wyoming.Conn, typ string, data any, payload []byte) error {
	ev, err := wyoming.NewEvent(typ, data, payload)
	if err != nil {
		return err
	}
	return conn.Write(ctx, ev)
}
