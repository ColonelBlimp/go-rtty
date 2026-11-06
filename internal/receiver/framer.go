// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

type framer struct {
	period, stop           float64
	previous, previousTime float64
	previousValid          bool
	markSince              float64
	markRun, ready         bool
	active                 bool
	start                  float64
	stage                  int // 0=start check; 1..5=data; 6=stop interval; 7=pending stop end
	code                   uint8
	prior                  Observation
	hasPrior               bool // previous frame awaiting an early next start's validation
}

func (f *framer) reject(d *Diagnostics) {
	d.Rejected++
	if f.hasPrior {
		d.Rejected++
	}
	f.active, f.hasPrior, f.ready, f.markRun = false, false, false, false
}

func (f *framer) begin(edge float64, d *Diagnostics) {
	f.active, f.ready = true, false
	f.start, f.stage, f.code = edge, 0, 0
	d.Candidates++
}

func (f *framer) sample(t, metric float64, valid bool, available int64, dst []Observation, d *Diagnostics) []Observation {
	return f.sampleWithEdge(t, metric, metric, valid, available, dst, d)
}

func (f *framer) sampleWithEdge(t, metric, edgeMetric float64, valid bool, available int64, dst []Observation, d *Diagnostics) []Observation {
	mark := metric >= 0
	edgeMark := edgeMetric >= 0
	falling := valid && f.previousValid && f.previous >= 0 && !edgeMark
	edge := t
	if falling {
		edge = f.previousTime + (t-f.previousTime)*f.previous/(f.previous-edgeMetric)
	}
	if !f.active {
		if falling && (f.ready || (f.markRun && edge-f.markSince >= f.period)) {
			f.begin(edge, d)
		}
	} else {
		age := (t - f.start) / f.period
		switch f.stage {
		case 0:
			if age >= .5 {
				if !valid || mark {
					f.reject(d)
					break
				}
				if f.hasPrior {
					f.prior.Available = available
					dst = append(dst, f.prior)
					f.hasPrior = false
				}
				f.stage = 1
			}
		case 1, 2, 3, 4, 5:
			if age >= float64(f.stage)+.5 {
				if !valid {
					f.reject(d)
					break
				}
				if mark {
					f.code |= 1 << (f.stage - 1)
				}
				f.stage++
			}
		case 6:
			if age >= 6.25 {
				if !valid || !mark {
					f.reject(d)
					break
				}
				if age >= 6.75 {
					f.stage = 7
				}
			}
		case 7:
			end := f.start + (6+f.stop)*f.period
			if !valid {
				f.reject(d)
				break
			}
			if falling {
				if edge < end-.2*f.period {
					f.reject(d)
					break
				}
				obs := Observation{Code: f.code, Start: f.start, End: end, Available: available}
				if edge < end {
					obs.End = edge
					f.prior, f.hasPrior = obs, true
				} else {
					dst = append(dst, obs)
				}
				f.begin(edge, d)
			} else if !edgeMark {
				f.reject(d)
			} else if t >= end {
				dst = append(dst, Observation{Code: f.code, Start: f.start, End: end, Available: available})
				f.active, f.ready = false, true
			}
		}
	}
	if valid && edgeMark {
		if !f.markRun {
			f.markSince, f.markRun = t, true
		}
	} else {
		f.markRun = false
	}
	if !valid {
		f.ready = false
	}
	f.previous, f.previousTime, f.previousValid = edgeMetric, t, valid
	return dst
}
