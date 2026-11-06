// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import "testing"

// This records the frozen detector's failures, not a passing reception gate.
func TestRetainedFadeFailureTrace(t *testing.T) {
	p, _ := loadNoise(t)
	m, _ := loadSelective(t)
	all := fixtures(t)
	for _, tc := range []struct {
		index int
		id    string
	}{{0, "mark-fade20"}, {8, "mark-fade20"}, {8, "space-fade20"}} {
		f := all[tc.index]
		var c selectiveCase
		for _, v := range m.Cases {
			if v.ID == tc.id {
				c = v
			}
		}
		pcm, _ := selectivePCM(f, wireLevels(t, f), c, m, 0)
		r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, false)}
		var out []Observation
		firstReject, stage, code := -1, -1, uint8(0)
		for i, s := range pcm {
			frame, rejected := r.base.frame, r.base.Diagnostics.Rejected
			out = r.Process([]int16{s}, out)
			if firstReject < 0 && rejected != r.base.Diagnostics.Rejected {
				firstReject, stage, code = i, frame.stage, frame.code
			}
		}
		codes, _ := codesAndText(out)
		if tc.index == 0 {
			if firstReject < 0 || stage != 6 || code != 4 || r.info.FirstCorrectionSample <= int64(firstReject) {
				t.Fatal("expected a rejected SPACE stop before correction was enabled")
			}
		} else if tc.id == "mark-fade20" {
			if firstReject != -1 || r.info.CorrectionSamples != 0 || edits(f.ExpectedCodes, codes).total() != 1 {
				t.Fatal("expected one accepted wrong code without active correction or framing rejection")
			}
		} else {
			if firstReject < 0 || r.info.CorrectionSamples != 0 || len(codes) >= len(f.ExpectedCodes) {
				t.Fatal("expected frame losses before any correction")
			}
		}
		t.Logf("%s/%s first_reject=%d stage=%d code=%d first_correction=%d codes=%v", f.ID, tc.id, firstReject, stage, code, r.info.FirstCorrectionSample, codes)
	}
}
