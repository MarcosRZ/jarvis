package audio

import (
	"context"
	"time"
)

// recordFrame is the unit for the speech level decision.
const recordFrame = 20 * time.Millisecond

// Source gives PCM audio. Capture is a Source.
type Source interface {
	Read(ctx context.Context, p []byte) (int, error)
}

// ReadFull reads exactly len(p) bytes from src.
func ReadFull(ctx context.Context, src Source, p []byte) error {
	for n := 0; n < len(p); {
		k, err := src.Read(ctx, p[n:])
		if err != nil {
			return err
		}
		n += k
	}
	return nil
}

type EndReason string

const (
	EndSilence  EndReason = "silence"
	EndMax      EndReason = "max-duration"
	EndNoSpeech EndReason = "no-speech"
)

type RecordLimits struct {
	// Max is the longest recording; longer speech is cut at Max.
	Max time.Duration
	// StartTimeout ends the recording with EndNoSpeech when no speech starts within it.
	StartTimeout time.Duration
	// EndSilence ends the recording after this much audio below SpeechLevel.
	EndSilence time.Duration
	// SpeechLevel is the RMS level in dBFS from which a frame counts as speech.
	SpeechLevel float64
}

// Recording is one spoken command. PCM is empty for EndNoSpeech.
type Recording struct {
	PCM         []byte
	Format      Format
	End         EndReason
	SpeechStart time.Duration
}

// Record reads src until the speaker stops, Max is reached, or no speech starts
// within StartTimeout. All durations count audio, not wall time. The audio is
// held in memory only, in one buffer of size Max.
func Record(ctx context.Context, src Source, format Format, limits RecordLimits) (Recording, error) {
	buf := make([]byte, format.BytesFor(limits.Max))
	frame := format.BytesFor(recordFrame)
	n := 0
	speechStart, speechEnd := -1, 0
	result := func(end EndReason) Recording {
		rec := Recording{PCM: buf[:n], Format: format, End: end, SpeechStart: format.Duration(max(speechStart, 0))}
		if end == EndNoSpeech {
			clear(buf)
			rec.PCM = nil
		}
		return rec
	}
	for n < len(buf) {
		end := min(n+frame, len(buf))
		if err := ReadFull(ctx, src, buf[n:end]); err != nil {
			clear(buf)
			return Recording{}, err
		}
		if RMSDBFS(buf[n:end]) >= limits.SpeechLevel {
			if speechStart < 0 {
				speechStart = n
			}
			speechEnd = end
		}
		n = end
		switch {
		case speechStart < 0 && format.Duration(n) >= limits.StartTimeout:
			return result(EndNoSpeech), nil
		case speechStart >= 0 && format.Duration(n-speechEnd) >= limits.EndSilence:
			return result(EndSilence), nil
		}
	}
	return result(EndMax), nil
}
