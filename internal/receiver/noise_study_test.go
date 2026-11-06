// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"crypto/sha256"
	"encoding/json"
	"flag"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"runtime"
	"slices"
	"testing"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

var noiseOutput = flag.String("receiver-noise-output", "", "export guard-band noise-aware trial results to a new file")

func loadNoise(t testing.TB) (noiseParameters, []byte) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/manifests/receiver-noise-aware-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var p noiseParameters
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Schema != 1 || p.Partition != "development" || p.GuardDistance != 340 || p.NoiseBits != 2 || p.PeakBits != 16 || p.ToNoise != 4 || p.Dominance != 2 || p.Qualification != .25 || p.MaxAge != 16 || p.Engage != 4 || p.Disengage != 2 || p.Floor != .0625 || p.MinNoise != 1e-8 || len(p.Candidates) != 2 {
		t.Fatal("unexpected noise-aware manifest")
	}
	return p, data
}

func newNoise(t testing.TB, f fixture, mode string, p noiseParameters, usos bool) *noiseTrial {
	t.Helper()
	r, err := newNoiseTrial(f.Config, usos, mode, p)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

type noiseRun struct {
	Group         string           `json:"group"`
	Fixture       string           `json:"fixture"`
	Case          string           `json:"case"`
	Seed          uint64           `json:"seed"`
	Candidate     string           `json:"candidate"`
	PCMHash       string           `json:"pcm_sha256"`
	Samples       int              `json:"samples"`
	Clipped       int              `json:"clipped_samples"`
	Codes         []int            `json:"codes"`
	TextOff       string           `json:"text_usos_off"`
	TextOn        string           `json:"text_usos_on"`
	CodeErrors    editCounts       `json:"code_errors"`
	TextOffErrors editCounts       `json:"text_usos_off_errors"`
	TextOnErrors  editCounts       `json:"text_usos_on_errors"`
	Rejected      int64            `json:"rejected_candidates"`
	Evidence      noiseDiagnostics `json:"evidence"`
	MaxDelayMS    *float64         `json:"max_availability_after_known_frame_ms,omitempty"`
}

type noiseControl struct {
	trialControl
	Evidence noiseDiagnostics `json:"evidence"`
}

type noiseReport struct {
	Status    string            `json:"status"`
	GoVersion string            `json:"go_version"`
	Inputs    map[string]string `json:"input_sha256"`
	Sources   map[string]string `json:"sources_sha256"`
	Results   []noiseRun        `json:"results"`
	Controls  []noiseControl    `json:"negative_controls"`
}

func TestNoiseAwareStudy(t *testing.T) {
	p, _ := loadNoise(t)
	report := noiseReport{Status: "development-characterization", GoVersion: runtime.Version(), Inputs: make(map[string]string), Sources: make(map[string]string)}
	for _, name := range []string{"noise_trial_test.go", "noise_study_test.go", "threshold_test.go", "selective_test.go", "channel_test.go", "receiver_test.go", "receiver.go", "framer.go", "text.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		report.Sources[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	for _, path := range []string{"manifests/receiver-noise-aware-v1.json", "manifests/receiver-selective-v1.json", "manifests/receiver-channel-v1.json", "manifests/independent.json", "reference/receiver-selective-v1.json", "reference/receiver-adaptive-edge-v2.json"} {
		data, err := os.ReadFile("../../testdata/" + path)
		if err != nil {
			t.Fatal(err)
		}
		report.Inputs[path] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	data, err := os.ReadFile("../../testdata/reference/receiver-selective-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var old selectiveReport
	if err := json.Unmarshal(data, &old); err != nil {
		t.Fatal(err)
	}
	m, _ := loadSelective(t)
	prior, _ := loadChannelManifest(t)
	all := fixtures(t)
	lookup := make(map[string]fixture)
	for _, f := range all {
		lookup[f.ID] = f
	}
	appendRun := func(f fixture, pcm []int16, clips int, group, id string, seed uint64, mode, expectedHash string) noiseRun {
		hash := pcmHash(pcm)
		if hash != expectedHash {
			t.Fatalf("%s/%s/%d input changed", f.ID, id, seed)
		}
		r := newNoise(t, f, mode, p, false)
		obs := r.Process(pcm, nil)
		codes, off := codesAndText(obs)
		on := renderPolicy(codes, true)
		row := noiseRun{Group: group, Fixture: f.ID, Case: id, Seed: seed, Candidate: mode, PCMHash: hash, Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.base.Diagnostics.Rejected, Evidence: r.info}
		if slices.Equal(codes, f.ExpectedCodes) {
			delay, aligned := 0.0, true
			for i, o := range obs {
				start := float64(f.Timing.Lead + i*(6*f.Timing.Bit+f.Timing.Stop))
				if math.Abs(o.Start-start) > .5*float64(f.Timing.Bit) {
					aligned = false
				}
				delay = math.Max(delay, (float64(o.Available)-start-float64(6*f.Timing.Bit+f.Timing.Stop))*1000/float64(f.SampleRate))
			}
			if aligned {
				delay = math.Round(delay*1000) / 1000
				row.MaxDelayMS = &delay
			}
		}
		partitioned := newNoise(t, f, mode, p, false)
		var got []Observation
		for pos := 0; pos < len(pcm); pos += 127 {
			got = partitioned.Process(pcm[pos:min(pos+127, len(pcm))], got)
			got = partitioned.Process(nil, got)
		}
		if !slices.Equal(got, obs) || !reflect.DeepEqual(partitioned, r) {
			t.Fatalf("%s/%s/%s chunk invariance", f.ID, id, mode)
		}
		report.Results = append(report.Results, row)
		return row
	}
	for _, mode := range p.Candidates {
		for _, c := range m.Cases {
			var ce, te, to int
			for _, before := range old.Results {
				if before.Candidate != "ratio" || before.Case != c.ID {
					continue
				}
				f := lookup[before.Fixture]
				pcm, clips := selectivePCM(f, wireLevels(t, f), c, m, before.Seed)
				row := appendRun(f, pcm, clips, "unequal-tone", c.ID, before.Seed, mode, before.PCMHash)
				ce += row.CodeErrors.total()
				te += row.TextOffErrors.total()
				to += row.TextOnErrors.total()
			}
			t.Logf("unequal-tone/%s/%s code_edits=%d text_off=%d text_on=%d", c.ID, mode, ce, te, to)
		}
		for _, c := range prior.Cases {
			var ce, te, to int
			for _, before := range old.Regression {
				if before.Candidate != "ratio" || before.Case != c.ID {
					continue
				}
				f := lookup[before.Fixture]
				pcm, clips := channelPCM(f, c, prior, before.Seed)
				f.Config.CenterHz += c.TuningHz
				row := appendRun(f, pcm, clips, "prior-matrix", c.ID, before.Seed, mode, before.PCMHash)
				ce += row.CodeErrors.total()
				te += row.TextOffErrors.total()
				to += row.TextOnErrors.total()
			}
			t.Logf("prior-matrix/%s/%s code_edits=%d text_off=%d text_on=%d", c.ID, mode, ce, te, to)
		}
	}
	for _, before := range old.Controls {
		if before.Candidate != "ratio" {
			continue
		}
		cfg := rtty.DefaultConfig()
		cfg.SampleRate, cfg.Polarity = before.Rate, before.Polarity
		mark, space, _ := cfg.ToneFrequencies()
		hz := mark
		if before.ID == "space" {
			hz = space
		}
		pcm := make([]int16, before.Samples)
		clipped := 0
		rng := rand.New(rand.NewPCG(before.Seed, 0x72747479))
		for i := range pcm {
			q := 0.0
			if before.ID == "noise" {
				q = math.RoundToEven(32768 * prior.NoiseSigma * rng.NormFloat64())
			} else if before.ID != "silence" {
				q = math.RoundToEven(1024 * math.Sin(2*math.Pi*hz*float64(i)/float64(before.Rate)))
			}
			if q < -32768 || q > 32767 {
				clipped++
			}
			pcm[i] = int16(max(-32768, min(32767, q)))
		}
		hash := pcmHash(pcm)
		if hash != before.PCMHash {
			t.Fatal("negative-control input changed")
		}
		for _, mode := range p.Candidates {
			r := newNoise(t, fixture{Config: cfg}, mode, p, false)
			obs := r.Process(pcm, nil)
			codes, text := codesAndText(obs)
			printable := 0
			for _, c := range []byte(text) {
				if c >= 32 && c <= 126 {
					printable++
				}
			}
			report.Controls = append(report.Controls, noiseControl{trialControl: trialControl{ID: before.ID, Rate: before.Rate, Polarity: before.Polarity, Seed: before.Seed, Candidate: mode, PCMHash: hash, Samples: len(pcm), Clipped: clipped, Codes: intCodes(codes), Text: text, Printable: printable, Controls: len(obs) - printable, Rejected: r.base.Diagnostics.Rejected}, Evidence: r.info})
			t.Logf("negative/%s/%d/%d/%d/%s printable=%d controls=%d correction_samples=%d", before.ID, before.Rate, before.Polarity, before.Seed, mode, printable, len(obs)-printable, r.info.CorrectionSamples)
		}
	}
	if *noiseOutput != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(*noiseOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, we := file.Write(append(data, '\n'))
		ce := file.Close()
		if we != nil || ce != nil {
			t.Fatalf("write report: %v %v", we, ce)
		}
		return
	}
	data, err = os.ReadFile("../../testdata/reference/receiver-noise-aware-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var retained noiseReport
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	report.GoVersion = retained.GoVersion
	if !reflect.DeepEqual(report, retained) {
		t.Fatal("noise-aware report changed; export and review differences")
	}
}
