package tts

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"jarvis/client/internal/wyoming"
)

var (
	ErrUnknownVoice   = errors.New("voice not offered by the speech service")
	ErrUnknownSpeaker = errors.New("speaker not offered by the voice")
)

// Check confirms that the service offers the configured speaker. Piper uses
// the default speaker for an unknown one and only logs a warning, so the
// client checks it with describe. With no speaker set, Check does nothing:
// Piper already answers an unknown voice with an error.
func (c *Client) Check(ctx context.Context) error {
	if c.cfg.Speaker == "" {
		return nil
	}
	info, err := wyoming.Describe(ctx, c.cfg.Addr, c.cfg.Timeouts)
	if err != nil {
		return err
	}
	for _, program := range info.TTS {
		for _, voice := range program.Voices {
			if voice.Name != c.cfg.Voice {
				continue
			}
			names := make([]string, len(voice.Speakers))
			for i, s := range voice.Speakers {
				if s.Name == c.cfg.Speaker {
					return nil
				}
				names[i] = s.Name
			}
			if len(names) == 0 {
				return fmt.Errorf("%w: %q has one speaker; leave the speaker empty", ErrUnknownSpeaker, c.cfg.Voice)
			}
			return fmt.Errorf("%w: %q for %q; available: %s", ErrUnknownSpeaker, c.cfg.Speaker, c.cfg.Voice, strings.Join(names, ", "))
		}
	}
	return fmt.Errorf("%w: %q", ErrUnknownVoice, c.cfg.Voice)
}
