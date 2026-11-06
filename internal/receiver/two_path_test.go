// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"math"
	"math/rand/v2"
)

type twoPathCase struct {
	ID    string   `json:"id"`
	Delay int      `json:"delay_ticks"`
	Echo  float64  `json:"echo"`
	Fade  bool     `json:"fade"`
	SNRDB *float64 `json:"snr_db"`
}

type twoPathManifest struct {
	Schema     int           `json:"schema_version"`
	Partition  string        `json:"partition"`
	Generator  string        `json:"generator"`
	Gain       float64       `json:"signal_gain"`
	Bandwidth  float64       `json:"snr_reference_bandwidth_hz"`
	Seeds      []uint64      `json:"noise_seeds"`
	Candidates []string      `json:"candidates"`
	Cases      []twoPathCase `json:"cases"`
}

// The real two-path convolution never receives bit labels or receiver output.
// Zeros outside the stored input model physical silence; N+D includes the
// complete delayed response rather than flushing the receiver with padding.
func twoPathPCM(f fixture, c twoPathCase, m twoPathManifest, seed uint64) ([]int16, int) {
	delay := c.Delay * (f.SampleRate / 12000)
	out := make([]int16, len(f.PCM)+delay)
	var power float64
	for _, s := range f.PCM {
		x := float64(s) / 32768 * m.Gain
		power += x * x
	}
	power /= float64(len(f.PCM))
	sigma := 0.0
	if c.SNRDB != nil {
		sigma = math.Sqrt(power * float64(f.SampleRate) / (2 * m.Bandwidth * math.Pow(10, *c.SNRDB/10)))
	}
	rng := rand.New(rand.NewPCG(seed, 0x74776f70617468))
	clips := 0
	for i := range out {
		direct, echo := 0.0, 0.0
		if i < len(f.PCM) {
			direct = float64(f.PCM[i]) / 32768
		}
		if i >= delay && i-delay < len(f.PCM) {
			echo = float64(f.PCM[i-delay]) / 32768
		}
		a := c.Echo
		if c.Fade {
			u := (float64(i)/float64(len(f.PCM)) - .25) / .5
			depth := 0.0
			if u >= 0 && u <= 1 {
				depth = (1 - math.Cos(2*math.Pi*u)) / 2
			}
			a *= depth
		}
		x := m.Gain * (direct + a*echo) / (1 + c.Echo)
		if c.SNRDB != nil {
			x += sigma * rng.NormFloat64()
		}
		q := math.RoundToEven(32768 * x)
		if q < -32768 || q > 32767 {
			clips++
		}
		out[i] = int16(max(-32768, min(32767, q)))
	}
	return out, clips
}

func twoPathGain(hz float64, delay int, rate int, echo float64) float64 {
	return math.Sqrt(1+echo*echo+2*echo*math.Cos(2*math.Pi*hz*float64(delay)/float64(rate))) / (1 + echo)
}
