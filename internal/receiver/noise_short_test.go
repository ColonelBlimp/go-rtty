// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import "math"

// shortNoiseTrial keeps the frozen long-evidence trial intact. Its evidence
// energies are divided by two so observe's noise tests reflect the half-bit
// correlators' doubled noise bandwidth. Peaks are restored to signal units
// when correcting the one-bit decisions.
type shortNoiseTrial struct{ *noiseTrial }

func (r *shortNoiseTrial) correction(m, s float64) float64 {
	n := max(r.noise, r.params.MinNoise)
	a, b := math.Sqrt(max(0, m-n)), math.Sqrt(max(0, s-n))
	p, q := math.Sqrt(max(0, 2*r.markPeak-2*n)), math.Sqrt(max(0, 2*r.spacePeak-2*n))
	return (a - b - .5*(p-q)) / max(p+q, 1e-8)
}

func (r *shortNoiseTrial) Process(pcm []int16, dst []Observation) []Observation {
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
		shortM, shortS := energyOf(b.front.edgeMark)/shortScale, energyOf(b.front.edgeSpace)/shortScale
		delayed := b.shortTotals[delayPos]
		b.shortTotals[delayPos] = shortM + shortS
		r.observe(shortM/2, shortS/2, n)
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
				metric = r.correction(m, s)
				total := m + s
				if oldHold > 0 || b.front.edgeHold > 0 {
					total = delayed
				}
				edge = r.correction(total*(1+edge)/2, total*(1-edge)/2)
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
