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

var pathOutput = flag.String("receiver-path-output", "", "export qualified-fall and two-path channel results to a new file")

type pathProcessor interface {
	Process([]int16, []Observation) []Observation
	Reset()
}
type pathDecoder struct {
	engine pathProcessor
	stats  *Diagnostics
	info   *noiseDiagnostics
}

func loadPath(t testing.TB) (twoPathManifest, float64) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/manifests/receiver-two-path-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var m twoPathManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Schema != 1 || m.Partition != "development" || m.Generator != "receiver-two-path-v1" || m.Gain != .0625 || len(m.Cases) != 9 || len(m.Candidates) != 3 {
		t.Fatal("unexpected two-path manifest")
	}
	data, err = os.ReadFile("../../testdata/manifests/receiver-fade-tracking-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var p struct {
		Schema int     `json:"schema_version"`
		Fall   float64 `json:"qualified_fall_bits"`
	}
	if err := json.Unmarshal(data, &p); err != nil {
		t.Fatal(err)
	}
	if p.Schema != 1 || p.Fall != 1 {
		t.Fatal("unexpected falling-level parameters")
	}
	return m, p.Fall
}

func pathReceiver(t testing.TB, f fixture, mode string, usos bool) pathDecoder {
	t.Helper()
	if mode == "ratio" {
		r := newReceiver(t, f.Config, usos)
		return pathDecoder{engine: r, stats: &r.Diagnostics}
	}
	p, _ := loadNoise(t)
	r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, usos)}
	if mode == "guarded-short" {
		return pathDecoder{engine: r, stats: &r.base.Diagnostics, info: &r.info}
	}
	if mode != "qualified-fall" {
		t.Fatalf("unknown path trial %q", mode)
	}
	_, fall := loadPath(t)
	v := &fadeTrackingTrial{shortNoiseTrial: r, fall: 1 - math.Exp(-1/(fall*r.base.frame.period))}
	return pathDecoder{engine: v, stats: &v.base.Diagnostics, info: &v.info}
}

func TestPathStudy(t *testing.T) {
	m, _ := loadPath(t)
	report := noiseReport{Status: "development-characterization", GoVersion: runtime.Version(), Inputs: make(map[string]string), Sources: make(map[string]string)}
	for _, name := range []string{"path_study_test.go", "two_path_test.go", "fade_tracking_test.go", "noise_short_test.go", "noise_trial_test.go", "noise_study_test.go", "threshold_test.go", "selective_test.go", "channel_test.go", "receiver_test.go", "receiver.go", "framer.go", "text.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		report.Sources[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	for _, name := range []string{"manifests/receiver-two-path-v1.json", "manifests/receiver-fade-tracking-v1.json", "manifests/receiver-noise-aware-v1.json", "manifests/receiver-noise-short-v1.json", "manifests/receiver-selective-v1.json", "manifests/receiver-channel-v1.json", "manifests/independent.json", "reference/receiver-noise-short-v1.json"} {
		data, err := os.ReadFile("../../testdata/" + name)
		if err != nil {
			t.Fatal(err)
		}
		report.Inputs[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	data, err := os.ReadFile("../../testdata/reference/receiver-noise-short-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var previous noiseReport
	if err := json.Unmarshal(data, &previous); err != nil {
		t.Fatal(err)
	}
	all := fixtures(t)
	lookup := make(map[string]fixture)
	for _, f := range all {
		lookup[f.ID] = f
	}
	sel, _ := loadSelective(t)
	prior, _ := loadChannelManifest(t)
	selCases := make(map[string]selectiveCase)
	for _, c := range sel.Cases {
		selCases[c.ID] = c
	}
	priorCases := make(map[string]channelCase)
	for _, c := range prior.Cases {
		priorCases[c.ID] = c
	}
	summary := make(map[string][3]int)
	run := func(f fixture, pcm []int16, clips int, group, id string, seed uint64, mode, wantHash string) {
		hash := pcmHash(pcm)
		if wantHash != "" && hash != wantHash {
			t.Fatal("path study input changed")
		}
		r := pathReceiver(t, f, mode, false)
		obs := r.engine.Process(pcm, nil)
		codes, off := codesAndText(obs)
		on := renderPolicy(codes, true)
		info := noiseDiagnostics{FirstCorrectionSample: -1}
		if r.info != nil {
			info = *r.info
		}
		row := noiseRun{Group: group, Fixture: f.ID, Case: id, Seed: seed, Candidate: mode, PCMHash: hash, Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.stats.Rejected, Evidence: info}
		if group != "two-path" && slices.Equal(codes, f.ExpectedCodes) {
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
		partitioned := pathReceiver(t, f, mode, false)
		var got []Observation
		for pos := 0; pos < len(pcm); pos += 127 {
			got = partitioned.engine.Process(pcm[pos:min(pos+127, len(pcm))], got)
			got = partitioned.engine.Process(nil, got)
		}
		if !slices.Equal(got, obs) || !reflect.DeepEqual(partitioned.engine, r.engine) {
			t.Fatalf("%s/%s/%s partition changed state", f.ID, id, mode)
		}
		report.Results = append(report.Results, row)
		key := group + "/" + id + "/" + mode
		s := summary[key]
		s[0] += row.CodeErrors.total()
		s[1] += row.TextOffErrors.total()
		s[2] += row.TextOnErrors.total()
		summary[key] = s
	}
	for _, before := range previous.Results {
		f := lookup[before.Fixture]
		var pcm []int16
		var clips int
		if before.Group == "unequal-tone" {
			pcm, clips = selectivePCM(f, wireLevels(t, f), selCases[before.Case], sel, before.Seed)
		} else {
			c := priorCases[before.Case]
			pcm, clips = channelPCM(f, c, prior, before.Seed)
			f.Config.CenterHz += c.TuningHz
		}
		run(f, pcm, clips, before.Group, before.Case, before.Seed, "qualified-fall", before.PCMHash)
	}
	for _, c := range m.Cases {
		seeds := []uint64{0}
		if c.SNRDB != nil {
			seeds = m.Seeds
		}
		for _, seed := range seeds {
			for _, f := range all {
				pcm, clips := twoPathPCM(f, c, m, seed)
				for _, mode := range m.Candidates {
					run(f, pcm, clips, "two-path", c.ID, seed, mode, "")
				}
			}
		}
	}
	for _, before := range previous.Controls {
		cfg := rtty.DefaultConfig()
		cfg.SampleRate, cfg.Polarity = before.Rate, before.Polarity
		mark, space, _ := cfg.ToneFrequencies()
		hz := mark
		if before.ID == "space" {
			hz = space
		}
		pcm := make([]int16, before.Samples)
		rng := rand.New(rand.NewPCG(before.Seed, 0x72747479))
		clips := 0
		for i := range pcm {
			q := 0.0
			if before.ID == "noise" {
				q = math.RoundToEven(32768 * prior.NoiseSigma * rng.NormFloat64())
			} else if before.ID != "silence" {
				q = math.RoundToEven(1024 * math.Sin(2*math.Pi*hz*float64(i)/float64(before.Rate)))
			}
			if q < -32768 || q > 32767 {
				clips++
			}
			pcm[i] = int16(max(-32768, min(32767, q)))
		}
		if pcmHash(pcm) != before.PCMHash {
			t.Fatal("path study control input changed")
		}
		r := pathReceiver(t, fixture{Config: cfg}, "qualified-fall", false)
		obs := r.engine.Process(pcm, nil)
		codes, text := codesAndText(obs)
		printable := 0
		for _, c := range []byte(text) {
			if c >= 32 && c <= 126 {
				printable++
			}
		}
		report.Controls = append(report.Controls, noiseControl{trialControl: trialControl{ID: before.ID, Rate: before.Rate, Polarity: before.Polarity, Seed: before.Seed, Candidate: "qualified-fall", PCMHash: before.PCMHash, Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), Text: text, Printable: printable, Controls: len(obs) - printable, Rejected: r.stats.Rejected}, Evidence: *r.info})
	}
	keys := make([]string, 0, len(summary))
	for k := range summary {
		keys = append(keys, k)
	}
	slices.Sort(keys)
	for _, k := range keys {
		t.Logf("%s code/text-off/text-on=%v", k, summary[k])
	}
	if *pathOutput != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(*pathOutput, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, we := file.Write(append(data, '\n'))
		ce := file.Close()
		if we != nil || ce != nil {
			t.Fatalf("write path report: %v %v", we, ce)
		}
		return
	}
	data, err = os.ReadFile("../../testdata/reference/receiver-two-path-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var retained noiseReport
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	report.GoVersion = retained.GoVersion
	if !reflect.DeepEqual(report, retained) {
		t.Fatal("path report differs; export and review")
	}
}
