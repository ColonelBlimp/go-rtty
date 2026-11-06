// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"crypto/sha256"
	"encoding/csv"
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

var selectiveOutput = flag.String("receiver-selective-output", "", "export a new selective-amplitude development report; existing files are refused")

type selectiveCase struct {
	ID      string   `json:"id"`
	MarkDB  float64  `json:"mark_db"`
	SpaceDB float64  `json:"space_db"`
	Fade    bool     `json:"fade"`
	SNRDB   *float64 `json:"snr_db"`
}

type selectiveManifest struct {
	Schema       int             `json:"schema_version"`
	Generator    string          `json:"generator"`
	Partition    string          `json:"partition"`
	Gain         float64         `json:"signal_gain"`
	TimeConstant float64         `json:"amplitude_time_constant_seconds"`
	Bandwidth    float64         `json:"snr_reference_bandwidth_hz"`
	Seeds        []uint64        `json:"noise_seeds"`
	ReleaseBits  float64         `json:"energy_peak_release_bits"`
	Floor        float64         `json:"normalization_floor_fraction"`
	Candidates   []string        `json:"candidates"`
	Cases        []selectiveCase `json:"cases"`
}

func loadSelective(t testing.TB) (selectiveManifest, []byte) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/manifests/receiver-selective-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var m selectiveManifest
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	if m.Schema != 1 || m.Generator != "receiver-selective-v1" || m.Partition != "development" || len(m.Cases) != 11 || m.ReleaseBits != 16 || m.Floor != 1.0/16 || len(m.Candidates) != 3 {
		t.Fatal("unexpected selective study manifest")
	}
	return m, data
}

// wireLevels uses the pre-existing CSV and independent transmitter timing,
// never the production text encoder/framer or recovered receiver decisions.
func wireLevels(t testing.TB, f fixture) []bool {
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
	levels := make([]bool, len(f.PCM))
	for i := range levels {
		levels[i] = true
	}
	pos := f.Timing.Lead
	run := func(mark bool, n int) {
		if pos+n > len(levels) {
			t.Fatal("transmit timing exceeds WAV")
		}
		for i := pos; i < pos+n; i++ {
			levels[i] = mark
		}
		pos += n
	}
	for _, code := range f.ExpectedCodes {
		run(false, f.Timing.Bit)
		for _, bit := range rows[int(code)+1][3] {
			run(bit == '1', f.Timing.Bit)
		}
		run(true, f.Timing.Stop)
	}
	if len(levels)-pos != f.Timing.Lead {
		t.Fatal("unexpected independent trailer")
	}
	return levels
}

func selectivePCM(f fixture, levels []bool, c selectiveCase, m selectiveManifest, seed uint64) ([]int16, int) {
	out := make([]int16, len(f.PCM))
	var power float64
	for _, s := range f.PCM {
		x := float64(s) / 32768 * m.Gain
		power += x * x
	}
	power /= float64(len(f.PCM))
	sigma := 0.0
	if c.SNRDB != nil {
		sigma = math.Sqrt(power * float64(f.SampleRate) / (2 * m.Bandwidth * math.Pow(10, *c.SNRDB/10)))
	}
	rng := rand.New(rand.NewPCG(seed, 0x73656c656374))
	alpha := 1 - math.Exp(-1/(m.TimeConstant*float64(f.SampleRate)))
	gain := math.Pow(10, -c.MarkDB/20)
	if c.Fade {
		gain = 1
	}
	clipped := 0
	for i, s := range f.PCM {
		db := c.SpaceDB
		if levels[i] {
			db = c.MarkDB
		}
		target := math.Pow(10, -db/20)
		if c.Fade {
			u := (float64(i)/float64(len(out)) - .25) / .5
			depth := 0.0
			if u >= 0 && u <= 1 {
				depth = (1 - math.Cos(2*math.Pi*u)) / 2
			}
			target = 1 - (1-target)*depth
		}
		gain += alpha * (target - gain)
		x := float64(s) / 32768 * m.Gain * gain
		if c.SNRDB != nil {
			x += sigma * rng.NormFloat64()
		}
		q := math.RoundToEven(x * 32768)
		if q < -32768 || q > 32767 {
			clipped++
		}
		out[i] = int16(max(-32768, min(32767, q)))
	}
	return out, clipped
}

type selectiveResult struct {
	Fixture       string     `json:"fixture"`
	Case          string     `json:"case"`
	Seed          uint64     `json:"seed"`
	Candidate     string     `json:"candidate"`
	PCMHash       string     `json:"pcm_sha256"`
	Clipped       int        `json:"clipped_samples"`
	Codes         []int      `json:"codes"`
	TextOff       string     `json:"text_usos_off"`
	TextOn        string     `json:"text_usos_on"`
	CodeErrors    editCounts `json:"code_errors"`
	TextOffErrors editCounts `json:"text_usos_off_errors"`
	TextOnErrors  editCounts `json:"text_usos_on_errors"`
	Rejected      int64      `json:"rejected_candidates"`
	MaxDelayMS    *float64   `json:"max_availability_after_known_frame_ms,omitempty"`
}

type selectiveReport struct {
	Status            string            `json:"status"`
	ManifestHash      string            `json:"manifest_sha256"`
	GoVersion         string            `json:"go_version"`
	Sources           map[string]string `json:"sources_sha256"`
	Results           []selectiveResult `json:"results"`
	Regression        []trialRegression `json:"prior_impairment_comparison"`
	Controls          []trialControl    `json:"negative_controls"`
	InputManifestHash string            `json:"independent_manifest_sha256"`
	PriorManifestHash string            `json:"prior_channel_manifest_sha256"`
	PriorReportHash   string            `json:"prior_receiver_report_sha256"`
}

type trialRegression struct {
	Fixture       string     `json:"fixture"`
	Case          string     `json:"case"`
	Seed          uint64     `json:"seed"`
	Candidate     string     `json:"candidate"`
	PCMHash       string     `json:"pcm_sha256"`
	Codes         []int      `json:"codes"`
	TextOff       string     `json:"text_usos_off"`
	TextOn        string     `json:"text_usos_on"`
	CodeErrors    editCounts `json:"code_errors"`
	TextOffErrors editCounts `json:"text_usos_off_errors"`
	TextOnErrors  editCounts `json:"text_usos_on_errors"`
	Rejected      int64      `json:"rejected_candidates"`
}

type trialControl struct {
	ID        string        `json:"id"`
	Rate      int           `json:"sample_rate"`
	Polarity  rtty.Polarity `json:"polarity"`
	Seed      uint64        `json:"seed"`
	Candidate string        `json:"candidate"`
	PCMHash   string        `json:"pcm_sha256"`
	Samples   int           `json:"samples"`
	Codes     []int         `json:"codes"`
	Text      string        `json:"text"`
	Printable int           `json:"printable"`
	Controls  int           `json:"controls"`
	Rejected  int64         `json:"rejected_candidates"`
	Clipped   int           `json:"clipped_samples"`
}

func renderPolicy(codes []uint8, usos bool) string {
	r := Receiver{usos: usos}
	text := ""
	for _, c := range codes {
		text += r.render(c)
	}
	return text
}

func trial(t testing.TB, f fixture, mode string, usos bool) *thresholdTrial {
	t.Helper()
	r, err := newThresholdTrial(f.Config, usos, mode)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestSelectiveStudy(t *testing.T) {
	m, data := loadSelective(t)
	report := selectiveReport{Status: "development-characterization", ManifestHash: fmt.Sprintf("%x", sha256.Sum256(data)), GoVersion: runtime.Version(), Sources: make(map[string]string)}
	for _, name := range []string{"threshold_test.go", "selective_test.go", "receiver.go", "framer.go", "text.go", "channel_test.go", "receiver_test.go"} {
		b, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		report.Sources[name] = fmt.Sprintf("%x", sha256.Sum256(b))
	}
	all := fixtures(t)
	for path, dest := range map[string]*string{
		"../../testdata/manifests/independent.json":               &report.InputManifestHash,
		"../../testdata/manifests/receiver-channel-v1.json":       &report.PriorManifestHash,
		"../../testdata/reference/receiver-adaptive-edge-v2.json": &report.PriorReportHash,
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		*dest = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	for _, c := range m.Cases {
		for _, mode := range m.Candidates {
			seeds := []uint64{0}
			if c.SNRDB != nil {
				seeds = m.Seeds
			}
			codeErrors, offErrors, onErrors := 0, 0, 0
			for _, seed := range seeds {
				for _, f := range all {
					pcm, clips := selectivePCM(f, wireLevels(t, f), c, m, seed)
					r := trial(t, f, mode, false)
					obs := r.Process(pcm, nil)
					codes, off := codesAndText(obs)
					_, on := codesAndText(trial(t, f, mode, true).Process(pcm, nil))
					result := selectiveResult{Fixture: f.ID, Case: c.ID, Seed: seed, Candidate: mode, PCMHash: pcmHash(pcm), Clipped: clips, Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.Diagnostics.Rejected}
					if slices.Equal(codes, f.ExpectedCodes) {
						delay, aligned := 0.0, true
						for i, o := range obs {
							start := float64(f.Timing.Lead + i*(6*f.Timing.Bit+f.Timing.Stop))
							if math.Abs(o.Start-start) > .5*float64(f.Timing.Bit) {
								aligned = false
							}
							end := start + float64(6*f.Timing.Bit+f.Timing.Stop)
							delay = math.Max(delay, (float64(o.Available)-end)*1000/float64(f.SampleRate))
						}
						if aligned {
							delay = math.Round(delay*1000) / 1000
							result.MaxDelayMS = &delay
						}
					}
					report.Results = append(report.Results, result)
					codeErrors += result.CodeErrors.total()
					offErrors += result.TextOffErrors.total()
					onErrors += result.TextOnErrors.total()
					partitioned := trial(t, f, mode, false)
					var got []Observation
					for pos := 0; pos < len(pcm); pos += 127 {
						got = partitioned.Process(pcm[pos:min(pos+127, len(pcm))], got)
					}
					if !slices.Equal(got, obs) || !reflect.DeepEqual(partitioned, r) {
						t.Fatalf("%s/%s/%s/%d chunk invariance failed", f.ID, c.ID, mode, seed)
					}
				}
			}
			t.Logf("%s/%s code_edits=%d text_off_edits=%d text_on_edits=%d", c.ID, mode, codeErrors, offErrors, onErrors)
		}
	}
	prior, _ := loadChannelManifest(t)
	data, err := os.ReadFile("../../testdata/reference/receiver-adaptive-edge-v2.json")
	if err != nil {
		t.Fatal(err)
	}
	var frozen channelReport
	if err := json.Unmarshal(data, &frozen); err != nil {
		t.Fatal(err)
	}
	for _, mode := range m.Candidates {
		index := 0
		var codeEdits, offEdits, onEdits int
		for _, c := range prior.Cases {
			seeds := []uint64{0}
			if c.SNRDB != nil {
				seeds = prior.Seeds
			}
			for _, seed := range seeds {
				for _, f := range all {
					pcm, _ := channelPCM(f, c, prior, seed)
					f.Config.CenterHz += c.TuningHz
					r := trial(t, f, mode, false)
					codes, off := codesAndText(r.Process(pcm, nil))
					on := renderPolicy(codes, true)
					row := trialRegression{Fixture: f.ID, Case: c.ID, Seed: seed, Candidate: mode, PCMHash: pcmHash(pcm), Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.Diagnostics.Rejected}
					if mode == "ratio" {
						old := frozen.Results[index]
						if row.Fixture != old.Fixture || row.Case != old.Case || row.Seed != old.Seed || row.PCMHash != old.PCMHash || !slices.Equal(row.Codes, old.Codes) || row.TextOff != old.TextOff || row.TextOn != old.TextOn || row.Rejected != old.Rejected {
							t.Fatalf("ratio trial does not reproduce current receiver: %s/%s/%d", f.ID, c.ID, seed)
						}
					}
					index++
					report.Regression = append(report.Regression, row)
					codeEdits += row.CodeErrors.total()
					offEdits += row.TextOffErrors.total()
					onEdits += row.TextOnErrors.total()
				}
			}
		}
		t.Logf("prior-matrix/%s code_edits=%d text_off_edits=%d text_on_edits=%d", mode, codeEdits, offEdits, onEdits)
	}
	for _, rate := range []int{12000, 48000} {
		for _, polarity := range []rtty.Polarity{rtty.MarkHigh, rtty.MarkLow} {
			cfg := rtty.DefaultConfig()
			cfg.SampleRate, cfg.Polarity = rate, polarity
			mark, space, _ := cfg.ToneFrequencies()
			for _, id := range []string{"silence", "mark", "space", "noise"} {
				seeds := []uint64{0}
				seconds := 1
				if id == "noise" {
					seeds = prior.Seeds
					seconds = prior.NoiseSeconds
				}
				for _, seed := range seeds {
					pcm := make([]int16, rate*seconds)
					clips := 0
					rng := rand.New(rand.NewPCG(seed, 0x72747479))
					hz := mark
					if id == "space" {
						hz = space
					}
					for i := range pcm {
						if id == "noise" {
							q := math.RoundToEven(32768 * prior.NoiseSigma * rng.NormFloat64())
							if q < -32768 || q > 32767 {
								clips++
							}
							pcm[i] = int16(max(-32768, min(32767, q)))
						} else if id != "silence" {
							pcm[i] = int16(math.RoundToEven(1024 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate))))
						}
					}
					for _, mode := range m.Candidates {
						r := trial(t, fixture{Config: cfg}, mode, false)
						obs := r.Process(pcm, nil)
						codes, text := codesAndText(obs)
						printable := 0
						for _, c := range []byte(text) {
							if c >= 32 && c <= 126 {
								printable++
							}
						}
						report.Controls = append(report.Controls, trialControl{ID: id, Rate: rate, Polarity: polarity, Seed: seed, Candidate: mode, PCMHash: pcmHash(pcm), Samples: len(pcm), Codes: intCodes(codes), Text: text, Printable: printable, Controls: len(obs) - printable, Rejected: r.Diagnostics.Rejected, Clipped: clips})
						t.Logf("negative/%s/%d/%d/%d/%s printable=%d controls=%d", id, rate, polarity, seed, mode, printable, len(obs)-printable)
					}
				}
			}
		}
	}
	if *selectiveOutput != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(*selectiveOutput, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
		if err != nil {
			t.Fatal(err)
		}
		_, writeErr := file.Write(append(data, '\n'))
		closeErr := file.Close()
		if writeErr != nil || closeErr != nil {
			t.Fatalf("report write: %v %v", writeErr, closeErr)
		}
		return
	}
	data, err = os.ReadFile("../../testdata/reference/receiver-selective-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var retained selectiveReport
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	report.GoVersion = retained.GoVersion
	if !reflect.DeepEqual(report, retained) {
		t.Fatal("selective study differs from retained report; export a new report and inspect differences")
	}
}
