// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"crypto/sha256"
	"encoding/binary"
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

var channelOutput = flag.String("receiver-study-output", "", "write a new development study JSON instead of checking the retained report")
var channelLegacy = flag.Bool("receiver-study-legacy", false, "reproduce the original rectangular detector baseline")

type channelCase struct {
	ID               string   `json:"id"`
	TuningHz         float64  `json:"tuning_hz,omitempty"`
	ClockPPM         float64  `json:"clock_ppm,omitempty"`
	SNRDB            *float64 `json:"snr_db,omitempty"`
	FadeDB           float64  `json:"fade_db,omitempty"`
	DropoutBits      float64  `json:"dropout_bits,omitempty"`
	ImpulseAmplitude float64  `json:"impulse_amplitude,omitempty"`
}

type channelManifest struct {
	SchemaVersion int           `json:"schema_version"`
	Generator     string        `json:"generator"`
	Partition     string        `json:"partition"`
	Description   string        `json:"description"`
	SignalGain    float64       `json:"signal_gain"`
	Bandwidth     float64       `json:"snr_reference_bandwidth_hz"`
	Seeds         []uint64      `json:"noise_seeds"`
	NoiseSeconds  int           `json:"noise_only_seconds"`
	NoiseSigma    float64       `json:"noise_only_sigma"`
	Cases         []channelCase `json:"cases"`
}

// The channel equations and boundary conventions are versioned with the
// manifest and documented in docs/RECEIVER_IMPAIRMENTS.md. These transforms
// impair independent audio; they never call EncodeText or FrameCodes.
func channelPCM(f fixture, c channelCase, m channelManifest, seed uint64) (pcm []int16, clipped int) {
	ratio := 1 + c.ClockPPM/1e6
	n := int(math.Floor(float64(len(f.PCM)-1)*ratio)) + 1
	pcm = make([]int16, n)
	x := make([]float64, n)
	var power float64
	for i := range x {
		pos := float64(i) / ratio
		lo := int(pos)
		fraction := pos - float64(lo)
		a, b := float64(f.PCM[lo]), float64(f.PCM[min(lo+1, len(f.PCM)-1)])
		x[i] = (a + (b-a)*fraction) / 32768 * m.SignalGain
		power += x[i] * x[i]
	}
	power /= float64(n)
	sigma := 0.0
	if c.SNRDB != nil {
		sigma = math.Sqrt(power * float64(f.SampleRate) / (2 * m.Bandwidth * math.Pow(10, *c.SNRDB/10)))
	}
	rng := rand.New(rand.NewPCG(seed, 0x72747479))
	gapStart := int(.4 * float64(n))
	gapEnd := gapStart + int(math.Round(c.DropoutBits*float64(f.SampleRate)*float64(f.BaudDenominator)/float64(f.BaudNumerator)))
	for i, v := range x {
		if c.FadeDB != 0 && i >= n*35/100 && i < n*65/100 {
			v *= math.Pow(10, -c.FadeDB/20)
		}
		if c.DropoutBits != 0 && i >= gapStart && i < gapEnd {
			v = 0
		}
		if c.SNRDB != nil {
			v += sigma * rng.NormFloat64()
		}
		// Two samples every 100 ms, alternating impulse sign.
		if c.ImpulseAmplitude != 0 && i%(f.SampleRate/10) < 2 {
			amplitude := c.ImpulseAmplitude
			if (i/(f.SampleRate/10))%2 != 0 {
				amplitude = -amplitude
			}
			v += amplitude
		}
		q := math.RoundToEven(v * 32768)
		if q < -32768 || q > 32767 {
			clipped++
		}
		pcm[i] = int16(max(-32768, min(32767, q)))
	}
	return pcm, clipped
}

type editCounts struct {
	Substitutions int `json:"substitutions"`
	Deletions     int `json:"deletions"`
	Insertions    int `json:"insertions"`
}

func (e editCounts) total() int { return e.Substitutions + e.Deletions + e.Insertions }

// Minimum edit distance with deterministic tie order: substitution, deletion,
// insertion. Bytes equal characters in the supported ASCII scoring repertoire.
func edits(want, got []uint8) editCounts {
	previous := make([]editCounts, len(got)+1)
	current := make([]editCounts, len(got)+1)
	for j := range previous {
		previous[j].Insertions = j
	}
	for i, a := range want {
		current[0] = editCounts{Deletions: i + 1}
		for j, b := range got {
			best := previous[j]
			if a != b {
				best.Substitutions++
			}
			del := previous[j+1]
			del.Deletions++
			ins := current[j]
			ins.Insertions++
			if del.total() < best.total() {
				best = del
			}
			if ins.total() < best.total() {
				best = ins
			}
			current[j+1] = best
		}
		previous, current = current, previous
	}
	return previous[len(got)]
}

func pcmHash(pcm []int16) string {
	data := make([]byte, 2*len(pcm))
	for i, v := range pcm {
		binary.LittleEndian.PutUint16(data[2*i:], uint16(v))
	}
	return fmt.Sprintf("%x", sha256.Sum256(data))
}

type channelResult struct {
	Fixture       string     `json:"fixture"`
	Case          string     `json:"case"`
	Seed          uint64     `json:"seed"`
	PCMHash       string     `json:"pcm_sha256"`
	Samples       int        `json:"samples"`
	Clipped       int        `json:"clipped_samples"`
	Codes         []int      `json:"codes"`
	MeasuredSNR   *float64   `json:"measured_snr_db,omitempty"`
	TextOff       string     `json:"text_usos_off"`
	TextOn        string     `json:"text_usos_on"`
	CodeErrors    editCounts `json:"code_errors"`
	TextOffErrors editCounts `json:"text_usos_off_errors"`
	TextOnErrors  editCounts `json:"text_usos_on_errors"`
	Rejected      int64      `json:"rejected_candidates"`
}

type noiseResult struct {
	Rate         int    `json:"sample_rate"`
	Seed         uint64 `json:"seed"`
	PCMHash      string `json:"pcm_sha256"`
	Observations int    `json:"observations"`
	Printable    int    `json:"printable_characters"`
	Controls     int    `json:"control_observations"`
	Clipped      int    `json:"clipped_samples"`
	Codes        []int  `json:"codes"`
	Text         string `json:"text"`
	Rejected     int64  `json:"rejected_candidates"`
}

type channelReport struct {
	Status          string            `json:"status"`
	Generator       string            `json:"generator"`
	ManifestHash    string            `json:"manifest_sha256"`
	SourceHash      string            `json:"generator_source_sha256"`
	GoVersion       string            `json:"go_version"`
	Receiver        string            `json:"receiver,omitempty"`
	ReceiverSources map[string]string `json:"receiver_sources_sha256,omitempty"`
	Results         []channelResult   `json:"results"`
	Noise           []noiseResult     `json:"noise_only"`
}

func loadChannelManifest(t testing.TB) (channelManifest, []byte) {
	t.Helper()
	data, err := os.ReadFile("../../testdata/manifests/receiver-channel-v1.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest channelManifest
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.SchemaVersion != 1 || manifest.Generator != "receiver-channel-v1" || manifest.Partition != "development" || manifest.SignalGain != .0625 || manifest.Bandwidth != 2500 || len(manifest.Cases) != 24 || len(manifest.Seeds) != 3 {
		t.Fatal("unexpected study configuration")
	}
	return manifest, data
}

func intCodes(codes []uint8) []int {
	out := make([]int, len(codes))
	for i, c := range codes {
		out[i] = int(c)
	}
	return out
}

// Measure the realized signal-to-added-error ratio from quantized PCM; this
// includes noise quantization and any clipping, unlike the target equation.
func measuredSNR(signal, noisy []int16, rate int, bandwidth float64) float64 {
	var signalPower, errorPower float64
	for i, s := range signal {
		a := float64(s) / 32768
		e := float64(int32(noisy[i])-int32(s)) / 32768
		signalPower += a * a
		errorPower += e * e
	}
	return 10 * math.Log10(signalPower/(errorPower*2*bandwidth/float64(rate)))
}

func TestChannelStudy(t *testing.T) {
	manifest, data := loadChannelManifest(t)
	source, err := os.ReadFile("channel_test.go")
	if err != nil {
		t.Fatal(err)
	}
	report := channelReport{Status: "development-characterization", Generator: manifest.Generator, ManifestHash: fmt.Sprintf("%x", sha256.Sum256(data)), SourceHash: fmt.Sprintf("%x", sha256.Sum256(source)), GoVersion: runtime.Version()}
	report.Receiver = "adaptive-edge-v2"
	if *channelLegacy {
		report.Receiver = "rectangular-v1"
	}
	report.ReceiverSources = make(map[string]string)
	for _, name := range []string{"receiver.go", "framer.go", "text.go"} {
		data, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		report.ReceiverSources[name] = fmt.Sprintf("%x", sha256.Sum256(data))
	}
	makeReceiver := func(cfg rtty.Config, usos bool) *Receiver {
		r, err := newWithTiming(cfg, usos, !*channelLegacy)
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	allFixtures := fixtures(t)
	for _, c := range manifest.Cases {
		seeds := []uint64{0}
		if c.SNRDB != nil {
			seeds = manifest.Seeds
		}
		var codeErrors, offErrors, onErrors, clipped, referenceCodes, referenceText int
		for _, seed := range seeds {
			for _, f := range allFixtures {
				pcm, clips := channelPCM(f, c, manifest, seed)
				cfg := f.Config
				cfg.CenterHz += c.TuningHz
				r := makeReceiver(cfg, false)
				obs := r.Process(pcm, nil)
				codes, off := codesAndText(obs)
				onReceiver := makeReceiver(cfg, true)
				_, on := codesAndText(onReceiver.Process(pcm, nil))
				result := channelResult{Fixture: f.ID, Case: c.ID, Seed: seed, PCMHash: pcmHash(pcm), Samples: len(pcm), Clipped: clips, Codes: intCodes(codes), TextOff: off, TextOn: on, CodeErrors: edits(f.ExpectedCodes, codes), TextOffErrors: edits([]byte(f.ExpectedOff), []byte(off)), TextOnErrors: edits([]byte(f.ExpectedOn), []byte(on)), Rejected: r.Diagnostics.Rejected}
				if c.SNRDB != nil {
					cleanCase := c
					cleanCase.SNRDB = nil
					clean, _ := channelPCM(f, cleanCase, manifest, seed)
					measured := math.Round(measuredSNR(clean, pcm, f.SampleRate, manifest.Bandwidth)*1000) / 1000
					result.MeasuredSNR = &measured
				}
				report.Results = append(report.Results, result)
				codeErrors += result.CodeErrors.total()
				offErrors += result.TextOffErrors.total()
				onErrors += result.TextOnErrors.total()
				clipped += clips
				referenceCodes += len(f.ExpectedCodes)
				referenceText += len(f.ExpectedOff)
				// Check identical observations AND complete final state even for
				// erroneous reception. One-sample impaired calls get a separate test.
				for _, chunk := range []int{f.SampleRate / 50, 127} {
					partitioned := makeReceiver(cfg, false)
					var got []Observation
					for pos := 0; pos < len(pcm); pos += chunk {
						got = partitioned.Process(pcm[pos:min(pos+chunk, len(pcm))], got)
						got = partitioned.Process(nil, got)
					}
					if !slices.Equal(got, obs) || !reflect.DeepEqual(partitioned, r) {
						t.Fatalf("%s/%s/%d: chunk %d changed output/state", f.ID, c.ID, seed, chunk)
					}
				}
			}
		}
		t.Logf("%s code_edits=%d/%d text_off_edits=%d/%d text_on_edits=%d/%d clipped=%d", c.ID, codeErrors, referenceCodes, offErrors, referenceText, onErrors, referenceText, clipped)
	}
	for _, rate := range []int{12000, 48000} {
		for _, seed := range manifest.Seeds {
			rng := rand.New(rand.NewPCG(seed, 0x72747479))
			pcm := make([]int16, rate*manifest.NoiseSeconds)
			clipped := 0
			for i := range pcm {
				v := math.RoundToEven(32768 * manifest.NoiseSigma * rng.NormFloat64())
				if v < -32768 || v > 32767 {
					clipped++
				}
				pcm[i] = int16(max(-32768, min(32767, v)))
			}
			cfg := rtty.DefaultConfig()
			cfg.SampleRate = rate
			r := makeReceiver(cfg, false)
			obs := r.Process(pcm, nil)
			printable, controls := 0, 0
			for _, o := range obs {
				if len(o.Text) == 1 && o.Text[0] >= 32 && o.Text[0] <= 126 {
					printable++
				} else {
					controls++
				}
			}
			codes, text := codesAndText(obs)
			report.Noise = append(report.Noise, noiseResult{Rate: rate, Seed: seed, PCMHash: pcmHash(pcm), Observations: len(obs), Printable: printable, Controls: controls, Clipped: clipped, Codes: intCodes(codes), Text: text, Rejected: r.Diagnostics.Rejected})
			t.Logf("noise_only rate=%d seed=%d seconds=%d printable=%d controls=%d", rate, seed, manifest.NoiseSeconds, printable, controls)
		}
	}
	if *channelOutput != "" {
		data, err := json.MarshalIndent(report, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		file, err := os.OpenFile(*channelOutput, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644)
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
	path := "../../testdata/reference/receiver-adaptive-edge-v2.json"
	if *channelLegacy {
		path = "../../testdata/reference/receiver-channel-v1.json"
	}
	data, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var retained channelReport
	if err := json.Unmarshal(data, &retained); err != nil {
		t.Fatal(err)
	}
	// Runtime version is provenance, not a claim that later Go versions fail.
	report.GoVersion = retained.GoVersion
	if *channelLegacy {
		// The original report predates the comparison harness. Keep that
		// historical provenance intact while checking every input/output field.
		report.SourceHash = retained.SourceHash
		report.Receiver, report.ReceiverSources = retained.Receiver, retained.ReceiverSources
	}
	if !reflect.DeepEqual(report, retained) {
		t.Fatal("study differs from retained development report; export a new report and review the difference")
	}
	if !*channelLegacy {
		data, err := os.ReadFile("../../testdata/reference/receiver-channel-v1.json")
		if err != nil {
			t.Fatal(err)
		}
		var baseline channelReport
		if err := json.Unmarshal(data, &baseline); err != nil {
			t.Fatal(err)
		}
		if baseline.ManifestHash != report.ManifestHash || len(baseline.Results) != len(report.Results) {
			t.Fatal("comparison conditions changed")
		}
		for i, current := range report.Results {
			before := baseline.Results[i]
			if current.Fixture != before.Fixture || current.Case != before.Case || current.Seed != before.Seed || current.PCMHash != before.PCMHash || current.Samples != before.Samples || current.Clipped != before.Clipped {
				t.Fatal("comparison input changed")
			}
			if current.CodeErrors.total() > before.CodeErrors.total() || current.TextOffErrors.total() > before.TextOffErrors.total() || current.TextOnErrors.total() > before.TextOnErrors.total() {
				t.Fatalf("reception regression on %s/%s/%d", current.Fixture, current.Case, current.Seed)
			}
		}
		if !reflect.DeepEqual(report.Noise, baseline.Noise) {
			t.Fatal("short noise-only observations changed; review before claiming a regression-free improvement")
		}
	}
}

func TestEditCounts(t *testing.T) {
	for _, c := range []struct {
		want, got string
		edits     editCounts
	}{
		{"abc", "abc", editCounts{}}, {"abc", "axc", editCounts{Substitutions: 1}},
		{"abc", "ac", editCounts{Deletions: 1}}, {"ac", "abc", editCounts{Insertions: 1}},
		{"abc", "xyz", editCounts{Substitutions: 3}}, {"", "abc", editCounts{Insertions: 3}},
		{"abc", "", editCounts{Deletions: 3}}, {"abc", "XabcY", editCounts{Insertions: 2}},
	} {
		if got := edits([]byte(c.want), []byte(c.got)); got != c.edits {
			t.Fatalf("edits(%q,%q)=%+v want%+v", c.want, c.got, got, c.edits)
		}
	}
}

func TestChannelCalibration(t *testing.T) {
	m, _ := loadChannelManifest(t)
	snr := 5.0
	for _, f := range fixtures(t) {
		clean, _ := channelPCM(f, channelCase{}, m, 7)
		noisy, clips := channelPCM(f, channelCase{SNRDB: &snr}, m, 7)
		measured := measuredSNR(clean, noisy, f.SampleRate, m.Bandwidth)
		if math.Abs(measured-snr) > .25 || clips != 0 {
			t.Fatalf("%s: target SNR %g, measured %g, clips %d", f.ID, snr, measured, clips)
		}
		identity := m
		identity.SignalGain = 1
		pcm, clips := channelPCM(f, channelCase{}, identity, 0)
		if clips != 0 || !slices.Equal(pcm, f.PCM) {
			t.Fatal("identity channel changed independent PCM")
		}
	}
	// A three-point ramp has independently known half-sample interpolates.
	f := fixture{PCM: []int16{0, 100, 200}, SampleRate: 12000, BaudNumerator: 909, BaudDenominator: 20}
	m.SignalGain = 1
	pcm, clips := channelPCM(f, channelCase{ClockPPM: 1e6}, m, 0)
	if clips != 0 || !slices.Equal(pcm, []int16{0, 50, 100, 150, 200}) {
		t.Fatalf("clock resampling: %v", pcm)
	}
}

func TestImpairedOneSampleAndRandomChunks(t *testing.T) {
	m, _ := loadChannelManifest(t)
	for _, c := range m.Cases {
		if c.ID != "awgn0" && c.ID != "two-bit-dropout" && c.ID != "combined-plus-plus" && c.ID != "common-fade20" {
			continue
		}
		for _, f := range fixtures(t) {
			pcm, _ := channelPCM(f, c, m, m.Seeds[0])
			cfg := f.Config
			cfg.CenterHz += c.TuningHz
			whole := newReceiver(t, cfg, true)
			want := whole.Process(pcm, nil)
			for _, random := range []bool{false, true} {
				r := newReceiver(t, cfg, true)
				var got []Observation
				rng := rand.New(rand.NewPCG(31, 79))
				for pos := 0; pos < len(pcm); {
					n := 1
					if random {
						n = min(1+rng.IntN(2048), len(pcm)-pos)
					}
					got = r.Process(pcm[pos:pos+n], got)
					pos += n
				}
				if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
					t.Fatalf("%s/%s/random%v changed output/state", f.ID, c.ID, random)
				}
			}
		}
	}
}
