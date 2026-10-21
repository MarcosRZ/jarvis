package tts

import (
	"context"
	"errors"
	"fmt"
	"time"

	"jarvis/client/internal/audio"
)

// defaultPlayMargin is added to the audio length to get the playback deadline.
const defaultPlayMargin = 2 * time.Second

var ErrPlaybackStuck = errors.New("playback did not finish in time")

// Player plays one response. *audio.Playback is a Player.
type Player interface {
	Write(ctx context.Context, pcm []byte) error
	Drain(ctx context.Context) error
	Close()
}

// Output opens a Player for format.
type Output func(format audio.Format) (Player, error)

type Speaker struct {
	client     *Client
	open       Output
	playMargin time.Duration
}

type Timing struct {
	Synthesis time.Duration
	Playback  time.Duration
	Audio     time.Duration
}

func NewSpeaker(client *Client, open Output) *Speaker {
	return &Speaker{client: client, open: open, playMargin: defaultPlayMargin}
}

// Check confirms the voice settings with the service; see Client.Check.
func (s *Speaker) Check(ctx context.Context) error {
	return s.client.Check(ctx)
}

// Say synthesizes text and plays it to the end. The output is opened for each
// response in the format of that response, and always closed. Playback that
// takes longer than the audio plus a margin ends with ErrPlaybackStuck.
func (s *Speaker) Say(ctx context.Context, text string) (Timing, error) {
	started := time.Now()
	speech, err := s.client.Synthesize(ctx, text)
	if err != nil {
		return Timing{}, err
	}
	timing := Timing{Synthesis: time.Since(started), Audio: speech.Format.Duration(len(speech.PCM))}

	timing.Playback, err = s.Play(ctx, speech.Format, speech.PCM)
	return timing, err
}

// Play plays pcm to the end, with the same output and deadline rules as Say.
// It returns the playback time.
func (s *Speaker) Play(ctx context.Context, format audio.Format, pcm []byte) (time.Duration, error) {
	player, err := s.open(format)
	if err != nil {
		return 0, fmt.Errorf("open audio output: %w", err)
	}
	defer player.Close()

	length := format.Duration(len(pcm))
	deadline := length + s.playMargin
	playCtx, cancel := context.WithTimeout(ctx, deadline)
	defer cancel()
	playing := time.Now()
	err = player.Write(playCtx, pcm)
	if err == nil {
		err = player.Drain(playCtx)
	}
	elapsed := time.Since(playing)
	switch {
	case err == nil:
		return elapsed, nil
	case ctx.Err() != nil:
		return elapsed, ctx.Err()
	case errors.Is(err, context.DeadlineExceeded):
		return elapsed, fmt.Errorf("%w: %v of audio after %v", ErrPlaybackStuck, length, deadline)
	default:
		return elapsed, fmt.Errorf("play audio: %w", err)
	}
}
