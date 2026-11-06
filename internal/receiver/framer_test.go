// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"encoding/csv"
	"os"
	"slices"
	"testing"
)

// Logical fixtures bypass the detector to isolate timing and rejection rules.
// Literal wire bits come from the independent CSV, not the production encoder.
func logicalFrames(t *testing.T, stop, actualStop, lead float64, codes []uint8, corruptStop bool) ([]Observation, Diagnostics) {
	t.Helper()
	file, err := os.Open("../../testdata/protocol/alphabet-us.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatal("invalid wire-bit CSV", err)
	}
	f := framer{period: 100, stop: stop}
	var d Diagnostics
	var out []Observation
	var now int64
	run := func(mark bool, bits float64) {
		metric := -1.0
		if mark {
			metric = 1
		}
		for range int(bits * 100) {
			out = f.sample(float64(now)+.5, metric, true, now, out, &d)
			now++
		}
	}
	run(true, lead)
	for _, code := range codes {
		run(false, 1)
		for _, bit := range rows[int(code)+1][3] {
			run(bit == '1', 1)
		}
		if corruptStop {
			run(false, actualStop)
		} else {
			run(true, actualStop)
		}
	}
	run(true, 2)
	return out, d
}

func TestLogicalFraming(t *testing.T) {
	var codes []uint8
	for code := range 32 {
		codes = append(codes, uint8(code))
	}
	for _, stop := range []float64{1.5, 2} {
		for _, actual := range []float64{stop - .2, stop - .19, stop, stop + .2, stop + 1} {
			obs, d := logicalFrames(t, stop, actual, 2, codes, false)
			got, _ := codesAndText(obs)
			if !slices.Equal(got, codes) || d.Rejected != 0 {
				t.Fatalf("stop%g actual%g: codes%v rejected%d", stop, actual, got, d.Rejected)
			}
		}
		for _, actual := range []float64{1, stop - .21} {
			obs, d := logicalFrames(t, stop, actual, 2, []uint8{31, 0}, false)
			for _, o := range obs {
				if o.Code == 31 {
					t.Fatalf("short stop accepted: stop%g actual%g", stop, actual)
				}
			}
			if d.Rejected == 0 {
				t.Fatal("short stop not diagnosed")
			}
		}
		obs, d := logicalFrames(t, stop, stop, 2, []uint8{31, 0}, true)
		if len(obs) != 0 || d.Rejected == 0 {
			t.Fatal("space stop accepted")
		}
	}
}

func TestLogicalAcquisition(t *testing.T) {
	for _, lead := range []float64{.5, .99, 1.01, 2} {
		obs, _ := logicalFrames(t, 1.5, 1.5, lead, []uint8{0}, false)
		if (len(obs) == 1) != (lead > 1) {
			t.Fatalf("lead%g produced%v", lead, obs)
		}
	}
}

func TestRenderEveryCode(t *testing.T) {
	file, err := os.Open("../../testdata/protocol/alphabet-us.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatal("invalid alphabet", err)
	}
	controls := map[string]string{"NUL": "", "BEL": "", "FIGS": "", "LTRS": "", "SPACE": " ", "CR": "\r", "LF": "\n", "QUOTE": "\"", "APOSTROPHE": "'", "COMMA": ","}
	for code, row := range rows[1:] {
		for column := 1; column <= 2; column++ {
			r := Receiver{figures: column == 2}
			want := row[column]
			if mapped, ok := controls[want]; ok {
				want = mapped
			}
			if got := r.render(uint8(code)); got != want {
				t.Fatalf("code%d figures%v: %q want%q", code, column == 2, got, want)
			}
		}
	}
}

func TestEarlyStartMustValidate(t *testing.T) {
	// A pending LTRS with its earliest permitted next edge. A brief space
	// pulse returns to mark before the half-bit check: reject both candidates.
	f := framer{period: 100, stop: 1.5, active: true, start: 0, stage: 7, code: 31, previous: 1, previousTime: 729.5, previousValid: true}
	var d Diagnostics
	out := f.sample(730.5, -1, true, 731, nil, &d)
	if len(out) != 0 || !f.hasPrior {
		t.Fatal("early edge prematurely emitted previous frame")
	}
	out = f.sample(780.5, 1, true, 781, out, &d)
	if len(out) != 0 || f.active || d.Rejected != 2 {
		t.Fatalf("failed start accepted: %v %+v", out, d)
	}
}
