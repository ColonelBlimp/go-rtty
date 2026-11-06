// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

// Package receiver is an experimental fixed-tuning receiver baseline, not the
// public streaming modem. It has no AFC, ATC, qualified squelch, or flush API.
// It consumes only real samples and never pads an incomplete final character.
package receiver

import (
	"math"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

// Observation retains raw codes, rendered text, and source-sample positions.
// Start and End compensate the correlator's nominal group delay. Available
// is the zero-based input sample at which the observation becomes available.
// Fractional positions are experimental estimates, not public timestamps.
type Observation struct {
	Code       uint8
	Text       string
	Start, End float64
	Available  int64
}

// Diagnostics are cumulative sample-clock counters, independent of calls.
type Diagnostics struct {
	Samples, Candidates, Rejected int64
}

type branch struct {
	osc, step complex128
	ring      []complex128
	sum       complex128
}

func newBranch(hz float64, rate, window int) branch {
	s, c := math.Sincos(-2 * math.Pi * hz / float64(rate))
	return branch{osc: 1, step: complex(c, s), ring: make([]complex128, window)}
}

func (b *branch) sample(x float64, pos int, normalize bool) float64 {
	v := complex(x, 0) * b.osc
	b.sum += v - b.ring[pos]
	b.ring[pos] = v
	b.osc *= b.step
	if normalize {
		b.osc /= complex(math.Hypot(real(b.osc), imag(b.osc)), 0)
	}
	return real(b.sum)*real(b.sum) + imag(b.sum)*imag(b.sum)
}

// Receiver owns all stream history. Calls on one instance must be serialized.
// Independent instances may run concurrently. Reset begins a new stream at 0.
type Receiver struct {
	mark, space           branch
	edgeMark, edgeSpace   branch
	edgeDelay             []float64
	edgePos               int
	powerRing             []float64
	powerLong, powerShort float64
	edgeHold              int
	pos                   int
	delay, scale          float64
	framer                framer
	usos, figures         bool
	Diagnostics           Diagnostics
}

// New validates the existing public profile. USOS controls only text rendering.
// The energy floor is a fixed numerical guard, not calibrated carrier squelch.
func New(cfg rtty.Config, usos bool) (*Receiver, error) {
	return newWithTiming(cfg, usos, true)
}

// newWithTiming retains the original detector for baseline comparisons.
func newWithTiming(cfg rtty.Config, usos, shortEdges bool) (*Receiver, error) {
	mark, space, err := cfg.ToneFrequencies()
	if err != nil {
		return nil, err
	}
	period := float64(cfg.SampleRate) * float64(cfg.Baud.Denominator) / float64(cfg.Baud.Numerator)
	window := int(math.Round(period))
	r := &Receiver{
		mark:   newBranch(mark, cfg.SampleRate, window),
		space:  newBranch(space, cfg.SampleRate, window),
		delay:  float64(window-1) / 2,
		scale:  float64(window * window),
		framer: framer{period: period, stop: float64(cfg.StopBits) / 2},
		usos:   usos,
	}
	if shortEdges {
		shortWindow := window / 2
		r.edgeMark = newBranch(mark, cfg.SampleRate, shortWindow)
		r.edgeSpace = newBranch(space, cfg.SampleRate, shortWindow)
		// Both supported rates/baud presets round to even window lengths.
		// Delay the short path to share the long path's source timeline.
		r.edgeDelay = make([]float64, (window-shortWindow)/2)
		r.powerRing = make([]float64, window)
	}
	return r, nil
}

// Reset discards filters, oscillator phase, pending frames, counters, and shift
// history while retaining the chosen settings and allocated filter storage.
func (r *Receiver) Reset() {
	for _, b := range []*branch{&r.mark, &r.space, &r.edgeMark, &r.edgeSpace} {
		if len(b.ring) == 0 {
			continue
		}
		clear(b.ring)
		b.sum, b.osc = 0, 1
	}
	clear(r.edgeDelay)
	clear(r.powerRing)
	r.powerLong, r.powerShort, r.edgeHold = 0, 0, 0
	r.edgePos = 0
	r.pos, r.figures = 0, false
	r.Diagnostics = Diagnostics{}
	r.framer = framer{period: r.framer.period, stop: r.framer.stop}
}

// Process appends observations to dst. Empty input is a no-op. Persistent
// filters, oscillator phase, and framing state never depend on call boundaries.
// A caller providing enough dst capacity incurs no steady-state allocations.
func (r *Receiver) Process(pcm []int16, dst []Observation) []Observation {
	for _, sample := range pcm {
		n := r.Diagnostics.Samples
		x := float64(sample) / 32768
		normalize := n%4096 == 4095
		m := r.mark.sample(x, r.pos, normalize)
		s := r.space.sample(x, r.pos, normalize)
		r.pos++
		if r.pos == len(r.mark.ring) {
			r.pos = 0
		}
		energy := (m + s) / r.scale
		metric := 0.0
		if m+s > 0 {
			metric = (m - s) / (m + s)
		}
		edgeMetric := metric
		if len(r.edgeDelay) != 0 {
			window, shortWindow := len(r.powerRing), len(r.edgeMark.ring)
			powerPos := int(n % int64(window))
			p := x * x
			r.powerLong += p - r.powerRing[powerPos]
			r.powerShort += p - r.powerRing[(powerPos+window-shortWindow)%window]
			r.powerRing[powerPos] = p
			// A 6 dB disparity between adjacent half-bit broadband power
			// windows identifies common gain steps in either direction.
			olderPower := r.powerLong - r.powerShort
			if n >= int64(window) && r.powerLong/float64(window) > 1e-8 && (r.powerShort < .25*olderPower || olderPower < .25*r.powerShort) {
				r.edgeHold = window
			}
			pos := int(n % int64(len(r.edgeMark.ring)))
			// Bound isolated impulses in the short timing path. Data and
			// energy gating continue to use the original, unclipped samples.
			limit := 4 * math.Sqrt(max(r.powerLong/float64(window), 1e-8))
			fastX := max(-limit, min(limit, x))
			m := r.edgeMark.sample(fastX, pos, normalize)
			s := r.edgeSpace.sample(fastX, pos, normalize)
			short := 0.0
			if m+s > 0 {
				short = (m - s) / (m + s)
			}
			if r.edgeHold > 0 {
				edgeMetric = r.edgeDelay[r.edgePos]
				r.edgeHold--
			}
			r.edgeDelay[r.edgePos] = short
			r.edgePos = (r.edgePos + 1) % len(r.edgeDelay)
		}
		t := float64(n) - r.delay
		if t >= 0 {
			before := len(dst)
			dst = r.framer.sampleWithEdge(t, metric, edgeMetric, energy > 1e-8, n, dst, &r.Diagnostics)
			for i := before; i < len(dst); i++ {
				dst[i].Text = r.render(dst[i].Code)
			}
		}
		r.Diagnostics.Samples++
	}
	return dst
}
