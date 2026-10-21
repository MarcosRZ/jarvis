package audio

import (
	"encoding/binary"
	"math"
	"time"
)

// BeepFormat is the format of Beep.PCM.
var BeepFormat = Format{SampleRate: 48000, Channels: 1}

// Beep is the wake-word sound: notes played one after the other, each faded
// in and out so that they do not click.
type Beep struct {
	Notes        []float64 // Hz
	NoteDuration time.Duration
	Fade         time.Duration
	Level        float64 // peak, dBFS
}

// DefaultBeep is A5 and E6, a short rising fifth.
var DefaultBeep = Beep{
	Notes:        []float64{880, 1318.5},
	NoteDuration: 70 * time.Millisecond,
	Fade:         8 * time.Millisecond,
	Level:        -14,
}

// PCM returns the beep in BeepFormat.
func (b Beep) PCM() []byte {
	amp := fullScale * math.Pow(10, b.Level/20)
	perNote := BeepFormat.BytesFor(b.NoteDuration) / BytesPerSample
	fade := max(1, BeepFormat.BytesFor(b.Fade)/BytesPerSample)
	pcm := make([]byte, 0, len(b.Notes)*perNote*BytesPerSample)
	for _, freq := range b.Notes {
		for i := range perNote {
			gain := min(1, float64(i)/float64(fade), float64(perNote-1-i)/float64(fade))
			v := amp * gain * math.Sin(2*math.Pi*freq*float64(i)/float64(BeepFormat.SampleRate))
			pcm = binary.LittleEndian.AppendUint16(pcm, uint16(int16(math.Round(v))))
		}
	}
	return pcm
}
