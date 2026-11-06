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

var shortNoiseOutput = flag.String("receiver-noise-short-output", "", "export short-evidence follow-up to a new report")

func TestNoiseShortEvidenceStudy(t *testing.T) {
	p, _ := loadNoise(t)
	report := noiseReport{Status: "development-characterization", GoVersion: runtime.Version(), Inputs: make(map[string]string), Sources: make(map[string]string)}
	data, err := os.ReadFile("../../testdata/manifests/receiver-noise-short-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var variant struct {
		Schema     int     `json:"schema_version"`
		Candidate  string  `json:"candidate"`
		Window     float64 `json:"evidence_window_bits"`
		Multiplier float64 `json:"evidence_noise_multiplier"`
	}
	if err := json.Unmarshal(data, &variant); err != nil {
		t.Fatal(err)
	}
	if variant.Schema != 1 || variant.Window != .5 || variant.Multiplier != 2 {
		t.Fatal("unexpected short-evidence definition")
	}
	report.Inputs["manifests/receiver-noise-short-v1.json"] = fmt.Sprintf("%x", sha256.Sum256(data))
	data, err = os.ReadFile("../../testdata/reference/receiver-noise-aware-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var baseline noiseReport
	if err := json.Unmarshal(data, &baseline); err != nil {
		t.Fatal(err)
	}
	report.Inputs["reference/receiver-noise-aware-v1.json"] = fmt.Sprintf("%x", sha256.Sum256(data))
	for k, v := range baseline.Inputs {
		report.Inputs[k] = v
	}
	for k, v := range baseline.Sources {
		report.Sources[k] = v
	}
	for _, name := range []string{"noise_short_test.go", "noise_short_study_test.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		report.Sources[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	m, _ := loadSelective(t)
	prior, _ := loadChannelManifest(t)
	lookup := make(map[string]fixture)
	for _, f := range fixtures(t) {
		lookup[f.ID] = f
	}
	selectiveCases := make(map[string]selectiveCase)
	for _, c := range m.Cases {
		selectiveCases[c.ID] = c
	}
	priorCases := make(map[string]channelCase)
	for _, c := range prior.Cases {
		priorCases[c.ID] = c
	}
	summary := make(map[string][3]int)
	for _, old := range baseline.Results {
		if old.Candidate != "guarded-offset" {
			continue
		}
		f := lookup[old.Fixture]
		var pcm []int16
		var clips int
		if old.Group == "unequal-tone" {
			pcm, clips = selectivePCM(f, wireLevels(t, f), selectiveCases[old.Case], m, old.Seed)
		} else {
			c := priorCases[old.Case]
			pcm, clips = channelPCM(f, c, prior, old.Seed)
			f.Config.CenterHz += c.TuningHz
		}
		if pcmHash(pcm) != old.PCMHash {
			t.Fatal("short-evidence input changed")
		}
		r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, false)}
		obs := r.Process(pcm, nil)
		codes, off := codesAndText(obs)
		on := renderPolicy(codes, true)
		row := noiseRun{Group: old.Group, Fixture: f.ID, Case: old.Case, Seed: old.Seed, Candidate: variant.Candidate, PCMHash: old.PCMHash, Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.base.Diagnostics.Rejected, Evidence: r.info}
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
		partitioned := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, false)}
		var got []Observation
		for pos := 0; pos < len(pcm); pos += 127 {
			got = partitioned.Process(pcm[pos:min(pos+127, len(pcm))], got)
		}
		if !slices.Equal(got, obs) || !reflect.DeepEqual(partitioned, r) {
			t.Fatal("short-evidence chunk invariance")
		}
		report.Results = append(report.Results, row)
		key := row.Group + "/" + row.Case
		s := summary[key]
		s[0] += row.CodeErrors.total()
		s[1] += row.TextOffErrors.total()
		s[2] += row.TextOnErrors.total()
		summary[key] = s
	}
	for _, old := range baseline.Controls {
		if old.Candidate != "guarded-offset" {
			continue
		}
		// The guarded baseline already pins this control's exact PCM recipe.
		cfg := rtty.DefaultConfig()
		cfg.SampleRate, cfg.Polarity = old.Rate, old.Polarity
		mark, space, _ := cfg.ToneFrequencies()
		hz := mark
		if old.ID == "space" {
			hz = space
		}
		pcm := make([]int16, old.Samples)
		rng := rand.New(rand.NewPCG(old.Seed, 0x72747479))
		clips := 0
		for i := range pcm {
			q := 0.0
			if old.ID == "noise" {
				q = math.RoundToEven(32768 * prior.NoiseSigma * rng.NormFloat64())
			} else if old.ID != "silence" {
				q = math.RoundToEven(1024 * math.Sin(2*math.Pi*hz*float64(i)/float64(old.Rate)))
			}
			if q < -32768 || q > 32767 {
				clips++
			}
			pcm[i] = int16(max(-32768, min(32767, q)))
		}
		if pcmHash(pcm) != old.PCMHash {
			t.Fatal("short-evidence control input changed")
		}
		r := &shortNoiseTrial{newNoise(t, fixture{Config: cfg}, "guarded-offset", p, false)}
		obs := r.Process(pcm, nil)
		codes, text := codesAndText(obs)
		printable := 0
		for _, c := range []byte(text) {
			if c >= 32 && c <= 126 {
				printable++
			}
		}
		report.Controls = append(report.Controls, noiseControl{trialControl: trialControl{ID: old.ID, Rate: old.Rate, Polarity: old.Polarity, Seed: old.Seed, Candidate: variant.Candidate, PCMHash: old.PCMHash, Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), Text: text, Printable: printable, Controls: len(obs) - printable, Rejected: r.base.Diagnostics.Rejected}, Evidence: r.info})
	}
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		t.Logf("%s code/text-off/text-on=%v", k, summary[k])
	}
	if *shortNoiseOutput != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(*shortNoiseOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, we := file.Write(append(data, '\n'))
		ce := file.Close()
		if we != nil || ce != nil {
			t.Fatalf("write report %v %v", we, ce)
		}
		return
	}
	data, err = os.ReadFile("../../testdata/reference/receiver-noise-short-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var retained noiseReport
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	report.GoVersion = retained.GoVersion
	if !reflect.DeepEqual(report, retained) {
		t.Fatal("short-evidence report changed; export and review")
	}
}
