// Package tts turns text into speech with a Wyoming text-to-speech service
// (Piper) and plays it.
package tts

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"jarvis/client/internal/audio"
	"jarvis/client/internal/wyoming"
)

const (
	maxTextBytes = 2000
	// maxSpeech bounds the audio held in memory for one response.
	maxSpeech = 60 * time.Second
)

var (
	ErrEmptyText      = errors.New("no text to speak")
	ErrTextTooLong    = fmt.Errorf("text is longer than %d bytes", maxTextBytes)
	ErrNoAudio        = errors.New("speech service returned no audio")
	ErrMalformedAudio = errors.New("malformed audio from the speech service")
	ErrSpeechTooLong  = fmt.Errorf("speech is longer than %v", maxSpeech)
)

type Config struct {
	Addr  string
	Voice string
	// Speaker is empty for the default speaker of Voice.
	Speaker  string
	Timeouts wyoming.Timeouts
}

type Client struct {
	cfg Config
}

// Speech is synthesized audio in the format that the service reported.
type Speech struct {
	Format audio.Format
	PCM    []byte
}

func New(cfg Config) *Client {
	return &Client{cfg: cfg}
}

// Synthesize returns the speech for text. It checks that the audio is 16-bit
// PCM in one format, in whole frames, and shorter than maxSpeech.
func (c *Client) Synthesize(ctx context.Context, text string) (Speech, error) {
	text = strings.TrimSpace(text)
	switch {
	case text == "":
		return Speech{}, ErrEmptyText
	case len(text) > maxTextBytes:
		return Speech{}, ErrTextTooLong
	}

	conn, err := wyoming.Dial(ctx, c.cfg.Addr, c.cfg.Timeouts)
	if err != nil {
		return Speech{}, err
	}
	defer conn.Close()
	request, err := wyoming.NewEvent(wyoming.TypeSynthesize, wyoming.Synthesize{Text: text, Voice: &wyoming.Voice{Name: c.cfg.Voice, Speaker: c.cfg.Speaker}}, nil)
	if err != nil {
		return Speech{}, err
	}
	if err := conn.Write(ctx, request); err != nil {
		return Speech{}, err
	}

	var (
		format  wyoming.AudioFormat
		speech  Speech
		started bool
		limit   int
	)
	for {
		ev, err := conn.Read(ctx)
		if err != nil {
			return Speech{}, err
		}
		if err := wyoming.CheckType(ev, wyoming.TypeAudioStart, wyoming.TypeAudioChunk, wyoming.TypeAudioStop); err != nil {
			return Speech{}, fmt.Errorf("synthesize at %s: %w", c.cfg.Addr, err)
		}
		switch ev.Type {
		case wyoming.TypeAudioStart:
			if started {
				return Speech{}, fmt.Errorf("%w: second audio-start", ErrMalformedAudio)
			}
			if err := ev.Decode(&format); err != nil {
				return Speech{}, fmt.Errorf("%w: %v", ErrMalformedAudio, err)
			}
			if format.Width != audio.BytesPerSample || format.Channels < 1 || format.Rate < 1 {
				return Speech{}, fmt.Errorf("%w: unsupported format %+v; want 16-bit samples", ErrMalformedAudio, format)
			}
			started = true
			speech.Format = audio.Format{SampleRate: format.Rate, Channels: format.Channels}
			limit = speech.Format.BytesFor(maxSpeech)
		case wyoming.TypeAudioChunk:
			var chunkFormat wyoming.AudioFormat
			if err := ev.Decode(&chunkFormat); err != nil {
				return Speech{}, fmt.Errorf("%w: %v", ErrMalformedAudio, err)
			}
			switch {
			case !started:
				return Speech{}, fmt.Errorf("%w: audio-chunk before audio-start", ErrMalformedAudio)
			case chunkFormat != format:
				return Speech{}, fmt.Errorf("%w: chunk format %+v differs from %+v", ErrMalformedAudio, chunkFormat, format)
			case len(ev.Payload)%speech.Format.FrameSize() != 0:
				return Speech{}, fmt.Errorf("%w: chunk of %d bytes is not whole frames", ErrMalformedAudio, len(ev.Payload))
			case len(speech.PCM)+len(ev.Payload) > limit:
				return Speech{}, ErrSpeechTooLong
			}
			speech.PCM = append(speech.PCM, ev.Payload...)
		case wyoming.TypeAudioStop:
			if len(speech.PCM) == 0 {
				return Speech{}, ErrNoAudio
			}
			return speech, nil
		}
	}
}
