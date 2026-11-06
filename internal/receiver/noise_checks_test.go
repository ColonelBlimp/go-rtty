// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"math"
	"math/rand/v2"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func TestNoiseGuardCalibration(t *testing.T) {
	p, _ := loadNoise(t)
	for _, rate := range []int{12000, 48000} {
		cfg := rtty.DefaultConfig()
		cfg.SampleRate = rate
		pcm := make([]int16, rate*5)
		rng := rand.New(rand.NewPCG(7, 0x72747479))
		for i := range pcm {
			pcm[i] = int16(math.RoundToEven(32768 * .03 * rng.NormFloat64()))
		}
		r := newNoise(t, fixture{Config: cfg}, "guarded-offset", p, false)
		r.Process(pcm, nil)
		measured := r.info.GuardEnergySum / float64(len(pcm))
		want := .03 * .03 / float64(len(r.guardLow.ring))
		if math.Abs(measured/want-1) > .2 {
			t.Fatalf("rate%d guard mean%g want%g", rate, measured, want)
		}
		t.Logf("rate%d white-noise guard mean=%g expected=%g", rate, measured, want)
		mark, space, _ := cfg.ToneFrequencies()
		for _, hz := range []float64{mark, space} {
			pcm := make([]int16, rate)
			for i := range pcm {
				pcm[i] = int16(math.RoundToEven(1024 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate))))
			}
			r := newNoise(t, fixture{Config: cfg}, "guarded-offset", p, false)
			obs := r.Process(pcm, nil)
			leak := r.info.GuardEnergySum / float64(len(pcm)) / (math.Pow(1024.0/32768, 2) / 4)
			if len(obs) != 0 || r.info.CorrectionSamples != 0 || leak >= .01 {
				t.Fatalf("tone%g leak%g events%v", hz, leak, obs)
			}
			t.Logf("rate%d tone%g mean guard leakage fraction=%g", rate, hz, leak)
		}
	}
}

func TestNoiseAwareBalancedFallback(t *testing.T) {
	p, _ := loadNoise(t)
	for _, f := range fixtures(t) {
		for _, mode := range p.Candidates {
			r := newNoise(t, f, mode, p, true)
			current := newReceiver(t, f.Config, true)
			got, want := r.Process(f.PCM, nil), current.Process(f.PCM, nil)
			if r.info.CorrectionSamples != 0 || r.info.FirstCorrectionSample != -1 || !slices.Equal(got, want) || r.base.Diagnostics != current.Diagnostics || !reflect.DeepEqual(r.base.frame, current.framer) {
				t.Fatalf("%s/%s changed balanced reception", f.ID, mode)
			}
		}
	}
}

func TestNoiseAwareStreamingReset(t *testing.T) {
	p, _ := loadNoise(t)
	m, _ := loadSelective(t)
	all := fixtures(t)
	for _, f := range []fixture{all[0], all[4]} {
		for _, c := range m.Cases {
			if c.ID != "mark-minus12-noise10" && c.ID != "space-minus20" && c.ID != "space-fade20" {
				continue
			}
			pcm, _ := selectivePCM(f, wireLevels(t, f), c, m, m.Seeds[0])
			for _, mode := range p.Candidates {
				whole := newNoise(t, f, mode, p, true)
				want := whole.Process(pcm, nil)
				for _, size := range []int{1, f.SampleRate / 50, 0} {
					r := newNoise(t, f, mode, p, true)
					rng := rand.New(rand.NewPCG(41, 83))
					var got []Observation
					for pos := 0; pos < len(pcm); {
						n := size
						if n == 0 {
							n = 1 + rng.IntN(2048)
						}
						n = min(n, len(pcm)-pos)
						got = r.Process(pcm[pos:pos+n], got)
						got = r.Process(nil, got)
						pos += n
					}
					if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
						t.Fatalf("%s/%s/%s chunk%d changed state", f.ID, c.ID, mode, size)
					}
				}
				r := newNoise(t, f, mode, p, true)
				r.Process(pcm[:len(pcm)/2], nil)
				if r.noise == 0 || r.markPeak == 0 || r.spacePeak == 0 {
					t.Fatal("test did not train guards and peaks")
				}
				r.Reset()
				fresh := newNoise(t, f, mode, p, true)
				if !reflect.DeepEqual(r, fresh) {
					t.Fatal("reset retained noise/peak/evidence state")
				}
				got := r.Process(pcm, nil)
				if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
					t.Fatal("reset changed subsequent stream")
				}
			}
		}
	}
}

func TestNoiseGuardUnavailableProfile(t *testing.T) {
	p, _ := loadNoise(t)
	cfg := rtty.DefaultConfig()
	cfg.CenterHz = 385
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	if r, err := newNoiseTrial(cfg, false, "guarded-offset", p); r != nil || err == nil {
		t.Fatal("accepted a profile without room for experimental guards")
	}
}

func BenchmarkNoiseAwareCandidates(b *testing.B) {
	p, _ := loadNoise(b)
	for _, f := range fixtures(b) {
		if !strings.Contains(f.ID, "mark-high-stop3") || strings.HasPrefix(f.ID, "usos") {
			continue
		}
		for _, mode := range p.Candidates {
			b.Run(f.ID+"/"+mode, func(b *testing.B) {
				r := newNoise(b, f, mode, p, false)
				out := make([]Observation, 0, len(f.ExpectedCodes)+1)
				chunk := f.SampleRate / 50
				b.ReportAllocs()
				b.SetBytes(int64(len(f.PCM) * 2))
				b.ResetTimer()
				for range b.N {
					r.Reset()
					out = out[:0]
					for pos := 0; pos < len(f.PCM); pos += chunk {
						out = r.Process(f.PCM[pos:min(pos+chunk, len(f.PCM))], out)
					}
					if len(out) != len(f.ExpectedCodes) {
						b.Fatal("lost clean observations")
					}
				}
				b.ReportMetric(b.Elapsed().Seconds()/float64(b.N)/(float64(len(f.PCM))/float64(f.SampleRate)), "RTF")
			})
		}
	}
}
