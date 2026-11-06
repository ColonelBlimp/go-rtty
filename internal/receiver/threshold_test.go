// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"fmt"
	"math"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

// thresholdTrial is a test-only detector experiment. It observes the current
// receiver's PCM-derived metrics and feeds an independent framer/renderer.
// No transmitted code or expected text is available to the detector.
// The production experiment and its frozen reports remain unchanged.
type thresholdTrial struct {
	front                               *Receiver
	frame                               framer
	text                                Receiver
	mode                                string
	peakMark, peakSpace, release, floor float64
	shortTotals                         []float64
	scratch                             []Observation
	Diagnostics                         Diagnostics
}

func newThresholdTrial(cfg rtty.Config, usos bool, mode string) (*thresholdTrial, error) {
	if mode != "ratio" && mode != "bounded-normalization" && mode != "envelope-offset" {
		return nil, fmt.Errorf("unsupported threshold trial %q", mode)
	}
	front, err := New(cfg, usos)
	if err != nil {
		return nil, err
	}
	period := front.framer.period
	return &thresholdTrial{front: front, frame: framer{period: period, stop: front.framer.stop}, text: Receiver{usos: usos}, mode: mode, release: math.Exp(-1 / (16 * period)), floor: 1.0 / 16, shortTotals: make([]float64, len(front.edgeDelay)), scratch: make([]Observation, 0, 2)}, nil
}

func (r *thresholdTrial) Reset() {
	r.front.Reset()
	r.frame = framer{period: r.frame.period, stop: r.frame.stop}
	r.text.figures = false
	r.peakMark, r.peakSpace = 0, 0
	r.Diagnostics = Diagnostics{}
	clear(r.shortTotals)
	r.scratch = r.scratch[:0]
}

func energyOf(b branch) float64 { return real(b.sum)*real(b.sum) + imag(b.sum)*imag(b.sum) }

func (r *thresholdTrial) correct(m, s float64) float64 {
	if r.mode == "envelope-offset" {
		a, b := math.Sqrt(m), math.Sqrt(s)
		p, q := math.Sqrt(r.peakMark), math.Sqrt(r.peakSpace)
		return (a - b - .5*(p-q)) / max(p+q, 1e-8)
	}
	peak := max(r.peakMark, r.peakSpace)
	a := m / max(r.peakMark, r.floor*peak, 1e-8)
	b := s / max(r.peakSpace, r.floor*peak, 1e-8)
	if a+b == 0 {
		return 0
	}
	return (a - b) / (a + b)
}

func (r *thresholdTrial) Process(pcm []int16, dst []Observation) []Observation {
	for _, sample := range pcm {
		n := r.Diagnostics.Samples
		delayPos := r.front.edgePos
		previousHold := r.front.edgeHold
		// The front's framer cannot affect its filters or transient detector.
		// Retain preallocated scratch so reference observations do not allocate.
		r.scratch = r.front.Process([]int16{sample}, r.scratch[:0])
		rawM, rawS := energyOf(r.front.mark), energyOf(r.front.space)
		m, s := rawM/r.front.scale, rawS/r.front.scale
		shortScale := float64(len(r.front.edgeMark.ring) * len(r.front.edgeMark.ring))
		shortTotal := (energyOf(r.front.edgeMark) + energyOf(r.front.edgeSpace)) / shortScale
		delayedTotal := r.shortTotals[delayPos]
		r.shortTotals[delayPos] = shortTotal
		r.peakMark = max(m, r.peakMark*r.release)
		r.peakSpace = max(s, r.peakSpace*r.release)
		t := float64(n) - r.front.delay
		if t >= 0 {
			metric := 0.0
			if rawM+rawS > 0 {
				metric = (rawM - rawS) / (rawM + rawS)
			}
			edge := r.front.framer.previous
			if r.mode != "ratio" {
				metric = r.correct(m, s)
				total := m + s
				if previousHold > 0 || r.front.edgeHold > 0 {
					total = delayedTotal
				}
				// Reconstruct the two energies from ratio and summed energy.
				edge = r.correct(total*(1+edge)/2, total*(1-edge)/2)
			}
			before := len(dst)
			dst = r.frame.sampleWithEdge(t, metric, edge, (rawM+rawS)/r.front.scale > 1e-8, n, dst, &r.Diagnostics)
			for i := before; i < len(dst); i++ {
				dst[i].Text = r.text.render(dst[i].Code)
			}
		}
		r.Diagnostics.Samples++
	}
	return dst
}
