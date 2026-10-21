package audio

import (
	"encoding/binary"
	"math"
	"time"
)

// BytesPerSample is fixed: all audio in the client is signed 16-bit
// little-endian PCM, the format of Wyoming audio chunks.
const BytesPerSample = 2

const fullScale = 32768

type Format struct {
	SampleRate int
	Channels   int
}

// SpeechFormat is the internal format for wake-word detection and speech recognition.
var SpeechFormat = Format{SampleRate: 16000, Channels: 1}

func (f Format) FrameSize() int {
	return f.Channels * BytesPerSample
}

// BytesFor returns the size of d of audio, rounded down to whole frames.
func (f Format) BytesFor(d time.Duration) int {
	frames := int64(d) * int64(f.SampleRate) / int64(time.Second)
	return int(frames) * f.FrameSize()
}

// Duration returns the play time of n bytes, counting whole frames only.
func (f Format) Duration(n int) time.Duration {
	frames := int64(n / f.FrameSize())
	return time.Duration(frames * int64(time.Second) / int64(f.SampleRate))
}

// RMSDBFS returns the RMS level of pcm in dB relative to full scale, or -Inf for silence.
func RMSDBFS(pcm []byte) float64 {
	var sum float64
	n := 0
	for i := 0; i+1 < len(pcm); i += BytesPerSample {
		s := float64(int16(binary.LittleEndian.Uint16(pcm[i:])))
		sum += s * s
		n++
	}
	if sum == 0 {
		return math.Inf(-1)
	}
	return 10 * math.Log10(sum/float64(n)/(fullScale*fullScale))
}

// PeakDBFS returns the peak level of pcm in dB relative to full scale, or -Inf for silence.
func PeakDBFS(pcm []byte) float64 {
	peak := 0
	for i := 0; i+1 < len(pcm); i += BytesPerSample {
		s := int(int16(binary.LittleEndian.Uint16(pcm[i:])))
		peak = max(peak, s, -s)
	}
	if peak == 0 {
		return math.Inf(-1)
	}
	return 20 * math.Log10(float64(peak)/fullScale)
}
