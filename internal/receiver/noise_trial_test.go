// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"fmt"
	"math"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

type noiseParameters struct {
	Schema        int      `json:"schema_version"`
	Partition     string   `json:"partition"`
	Candidates    []string `json:"candidates"`
	GuardDistance float64  `json:"guard_distance_from_nearest_tone_hz"`
	NoiseBits     float64  `json:"noise_smoothing_bits"`
	MinNoise      float64  `json:"minimum_noise_energy"`
	PeakBits      float64  `json:"peak_release_bits"`
	ToNoise       float64  `json:"branch_to_noise_ratio"`
	Dominance     float64  `json:"dominance_energy_ratio"`
	Qualification float64  `json:"qualification_bits"`
	MaxAge        float64  `json:"evidence_max_age_bits"`
	Engage        float64  `json:"engage_power_ratio"`
	Disengage     float64  `json:"disengage_power_ratio"`
	Floor         float64  `json:"normalization_floor_fraction"`
}

type noiseDiagnostics struct {
	CorrectionSamples     int64    `json:"correction_samples"`
	QualifiedMarkSamples  int64    `json:"qualified_mark_samples"`
	QualifiedSpaceSamples int64    `json:"qualified_space_samples"`
	GuardEnergySum        float64  `json:"guard_energy_sum"`
	FirstCorrectionSample int64    `json:"first_correction_sample"`
	GateActivations       int64    `json:"gate_activations"`
	RejectedAtStage       [8]int64 `json:"rejected_at_stage"`
}

// noiseTrial is test-only. Neither guards nor evidence gates use transmitted
// codes, a transcript, or the weaker data branch as a noise measurement.
type noiseTrial struct {
	base                  *thresholdTrial
	guardLow, guardHigh   branch
	params                noiseParameters
	mode                  string
	noise, alpha, release float64
	markPeak, spacePeak   float64
	markRun, spaceRun     int64
	markSeen, spaceSeen   bool
	lastMark, lastSpace   int64
	engaged               bool
	info                  noiseDiagnostics
}

func newNoiseTrial(cfg rtty.Config, usos bool, mode string, p noiseParameters) (*noiseTrial, error) {
	if mode != "guarded-offset" && mode != "guarded-normalization" {
		return nil, fmt.Errorf("unknown noise trial %q", mode)
	}
	b, err := newThresholdTrial(cfg, usos, "ratio")
	if err != nil {
		return nil, err
	}
	low, high := cfg.CenterHz-cfg.ShiftHz/2-p.GuardDistance, cfg.CenterHz+cfg.ShiftHz/2+p.GuardDistance
	if low < 300 || high > float64(cfg.SampleRate)/2-300 {
		return nil, fmt.Errorf("noise trial guards outside experimental passband")
	}
	window := len(b.front.mark.ring)
	return &noiseTrial{base: b, guardLow: newBranch(low, cfg.SampleRate, window), guardHigh: newBranch(high, cfg.SampleRate, window), params: p, mode: mode, alpha: 1 - math.Exp(-1/(p.NoiseBits*b.frame.period)), release: math.Exp(-1 / (p.PeakBits * b.frame.period)), info: noiseDiagnostics{FirstCorrectionSample: -1}}, nil
}

func (r *noiseTrial) Reset() {
	r.base.Reset()
	for _, b := range []*branch{&r.guardLow, &r.guardHigh} {
		clear(b.ring)
		b.sum, b.osc = 0, 1
	}
	r.noise, r.markPeak, r.spacePeak = 0, 0, 0
	r.markRun, r.spaceRun, r.lastMark, r.lastSpace = 0, 0, 0, 0
	r.markSeen, r.spaceSeen, r.engaged = false, false, false
	r.info = noiseDiagnostics{FirstCorrectionSample: -1}
}

func (r *noiseTrial) observe(m, s float64, n int64) {
	p := r.params
	noise := max(r.noise, p.MinNoise)
	r.markPeak *= r.release
	r.spacePeak *= r.release
	qualified := int64(math.Ceil(p.Qualification * r.base.frame.period))
	if m > p.ToNoise*noise && m > p.Dominance*s {
		r.markPeak = max(r.markPeak, m)
		r.markRun++
		if r.markRun >= qualified {
			r.markSeen = true
			r.lastMark = n
			r.info.QualifiedMarkSamples++
		}
	} else {
		r.markRun = 0
	}
	if s > p.ToNoise*noise && s > p.Dominance*m {
		r.spacePeak = max(r.spacePeak, s)
		r.spaceRun++
		if r.spaceRun >= qualified {
			r.spaceSeen = true
			r.lastSpace = n
			r.info.QualifiedSpaceSamples++
		}
	} else {
		r.spaceRun = 0
	}
	lo, hi := min(r.markPeak, r.spacePeak), max(r.markPeak, r.spacePeak)
	recent := float64(n-r.lastMark) < p.MaxAge*r.base.frame.period && float64(n-r.lastSpace) < p.MaxAge*r.base.frame.period
	if !r.markSeen || !r.spaceSeen || !recent || lo <= p.ToNoise*noise {
		r.engaged = false
	} else if r.engaged {
		if hi <= p.Disengage*lo {
			r.engaged = false
		}
	} else if hi >= p.Engage*lo {
		r.engaged = true
		r.info.GateActivations++
	}
}

func (r *noiseTrial) correct(m, s float64) float64 {
	n := max(r.noise, r.params.MinNoise)
	m, s = max(0, m-n), max(0, s-n)
	p, q := max(0, r.markPeak-n), max(0, r.spacePeak-n)
	if r.mode == "guarded-offset" {
		a, b := math.Sqrt(m), math.Sqrt(s)
		x, y := math.Sqrt(p), math.Sqrt(q)
		return (a - b - .5*(x-y)) / max(x+y, 1e-8)
	}
	peak := max(p, q)
	a, b := m/max(p, r.params.Floor*peak, n), s/max(q, r.params.Floor*peak, n)
	if a+b == 0 {
		return 0
	}
	return (a - b) / (a + b)
}

func (r *noiseTrial) Process(pcm []int16, dst []Observation) []Observation {
	b := r.base
	for _, sample := range pcm {
		n := b.Diagnostics.Samples
		pos := b.front.pos
		x := float64(sample) / 32768
		gl := r.guardLow.sample(x, pos, n%4096 == 4095)
		gh := r.guardHigh.sample(x, pos, n%4096 == 4095)
		guard := (gl + gh) / (2 * b.front.scale)
		r.noise += r.alpha * (guard - r.noise)
		r.info.GuardEnergySum += guard
		delayPos, oldHold := b.front.edgePos, b.front.edgeHold
		b.scratch = b.front.Process([]int16{sample}, b.scratch[:0])
		rawM, rawS := energyOf(b.front.mark), energyOf(b.front.space)
		m, s := rawM/b.front.scale, rawS/b.front.scale
		shortScale := float64(len(b.front.edgeMark.ring) * len(b.front.edgeMark.ring))
		totalShort := (energyOf(b.front.edgeMark) + energyOf(b.front.edgeSpace)) / shortScale
		delayed := b.shortTotals[delayPos]
		b.shortTotals[delayPos] = totalShort
		r.observe(m, s, n)
		t := float64(n) - b.front.delay
		if t >= 0 {
			metric := 0.0
			if rawM+rawS > 0 {
				metric = (rawM - rawS) / (rawM + rawS)
			}
			edge := b.front.framer.previous
			if r.engaged {
				if r.info.CorrectionSamples == 0 {
					r.info.FirstCorrectionSample = n
				}
				r.info.CorrectionSamples++
				metric = r.correct(m, s)
				total := m + s
				if oldHold > 0 || b.front.edgeHold > 0 {
					total = delayed
				}
				edge = r.correct(total*(1+edge)/2, total*(1-edge)/2)
			}
			before := len(dst)
			rejected, stage := b.Diagnostics.Rejected, b.frame.stage
			dst = b.frame.sampleWithEdge(t, metric, edge, (rawM+rawS)/b.front.scale > 1e-8, n, dst, &b.Diagnostics)
			r.info.RejectedAtStage[stage] += b.Diagnostics.Rejected - rejected
			for i := before; i < len(dst); i++ {
				dst[i].Text = b.text.render(dst[i].Code)
			}
		}
		b.Diagnostics.Samples++
	}
	return dst
}
