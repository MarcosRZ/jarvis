package audio

import (
	"context"
	"encoding/binary"
	"math"
)

// HighPass removes low-frequency rumble from a Source with two cascaded
// second-order filters. It is used only on the wake-word audio, so recordings
// and speech recognition keep the original signal.
//
// The source must be 16-bit mono PCM, like SpeechFormat.
type HighPass struct {
	src    Source
	stages [2]biquad
	bypass bool
}

type biquad struct {
	b0, b1, b2 float64
	a1, a2     float64
	x1, x2     float64
	y1, y2     float64
}

// NewHighPass wraps src. A cutoff of 0 or an invalid sample rate passes the
// audio through unchanged.
func NewHighPass(src Source, cutoffHz float64, sampleRate int) *HighPass {
	h := &HighPass{src: src}
	if cutoffHz <= 0 || sampleRate <= 0 || cutoffHz >= float64(sampleRate)/2 {
		h.bypass = true
		return h
	}
	coefficients := highPassCoefficients(cutoffHz, float64(sampleRate))
	h.stages[0] = biquad{
		b0: coefficients.b0, b1: coefficients.b1, b2: coefficients.b2,
		a1: coefficients.a1, a2: coefficients.a2,
	}
	h.stages[1] = h.stages[0]
	return h
}

// Read reads from the source and filters the samples in place.
func (h *HighPass) Read(ctx context.Context, p []byte) (int, error) {
	n, err := h.src.Read(ctx, p)
	if n > 0 && !h.bypass {
		h.filter(p[:n])
	}
	return n, err
}

func (h *HighPass) filter(p []byte) {
	for i := 0; i+1 < len(p); i += 2 {
		x := float64(int16(binary.LittleEndian.Uint16(p[i : i+2])))
		for stage := range h.stages {
			x = h.stages[stage].process(x)
		}
		x = math.Round(x)
		if x > 32767 {
			x = 32767
		} else if x < -32768 {
			x = -32768
		}
		binary.LittleEndian.PutUint16(p[i:i+2], uint16(int16(x)))
	}
}

func (b *biquad) process(x float64) float64 {
	y := b.b0*x + b.b1*b.x1 + b.b2*b.x2 - b.a1*b.y1 - b.a2*b.y2
	b.x2, b.x1 = b.x1, x
	b.y2, b.y1 = b.y1, y
	return y
}

// highPassCoefficients returns an RBJ second-order high-pass with Q = 1/sqrt(2).
func highPassCoefficients(cutoffHz, sampleRate float64) biquad {
	w0 := 2 * math.Pi * cutoffHz / sampleRate
	cosW, sinW := math.Cos(w0), math.Sin(w0)
	alpha := sinW / (2 * math.Sqrt2 / 2)
	b0 := (1 + cosW) / 2
	b1 := -(1 + cosW)
	b2 := b0
	a0 := 1 + alpha
	a1 := -2 * cosW
	a2 := 1 - alpha
	return biquad{
		b0: b0 / a0,
		b1: b1 / a0,
		b2: b2 / a0,
		a1: a1 / a0,
		a2: a2 / a0,
	}
}
