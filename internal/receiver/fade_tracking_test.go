// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import "math"

type fadeTrackingTrial struct {
	*shortNoiseTrial
	fall float64
}

// Follow a falling level only after a branch's existing evidence rule has
// qualified it. Decay during an absent branch still uses the original release.
func (r *fadeTrackingTrial) observeFalling(m, s float64, n int64) {
	p := r.params
	noise := max(r.noise, p.MinNoise)
	r.markPeak *= r.release
	r.spacePeak *= r.release
	qualified := int64(math.Ceil(p.Qualification * r.base.frame.period))
	if m > p.ToNoise*noise && m > p.Dominance*s {
		r.markPeak = max(r.markPeak, m)
		r.markRun++
		if r.markRun >= qualified {
			r.markPeak += r.fall * (m - r.markPeak)
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
			r.spacePeak += r.fall * (s - r.spacePeak)
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

func (r *fadeTrackingTrial) Process(pcm []int16, dst []Observation) []Observation {
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
		r.observeFalling(shortM/2, shortS/2, n)
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
