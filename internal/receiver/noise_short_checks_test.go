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

func TestHalfBitNoiseBandwidth(t *testing.T) {
	for _, rate := range []int{12000, 48000} {
		window := int(math.Round(float64(rate) * 11 / 500))
		long, short := newBranch(1075, rate, window), newBranch(1075, rate, window/2)
		rng := rand.New(rand.NewPCG(7, 0x6e6f697365))
		var a, b float64
		for i := range rate * 5 {
			x := .03 * rng.NormFloat64()
			a += long.sample(x, i%window, i%4096 == 4095) / float64(window*window)
			b += short.sample(x, i%(window/2), i%4096 == 4095) / float64(window*window/4)
		}
		if math.Abs(b/a-2) > .4 {
			t.Fatalf("rate%d half/whole noise energy ratio%g", rate, b/a)
		}
		t.Logf("rate%d measured half/whole noise energy ratio=%g", rate, b/a)
	}
}

func TestNoiseShortIsolatedSpaceQualification(t *testing.T) {
	p, _ := loadNoise(t)
	m, _ := loadSelective(t)
	for _, f := range fixtures(t) {
		pcm, _ := selectivePCM(f, wireLevels(t, f), selectiveCase{SpaceDB: 20}, m, 0)
		for _, usos := range []bool{false, true} {
			r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, usos)}
			codes, text := codesAndText(r.Process(pcm, nil))
			want := f.ExpectedOff
			if usos {
				want = f.ExpectedOn
			}
			if !slices.Equal(codes, f.ExpectedCodes) || text != want {
				t.Fatalf("%s short evidence lost isolated weak start", f.ID)
			}
			if r.info.FirstCorrectionSample < 0 || r.info.FirstCorrectionSample >= int64(f.Timing.Lead+f.Timing.Bit) {
				t.Fatalf("%s qualified after first real start bit: %d", f.ID, r.info.FirstCorrectionSample)
			}
		}
	}
}

func TestNoiseShortStreamingReset(t *testing.T) {
	p, _ := loadNoise(t)
	m, _ := loadSelective(t)
	all := fixtures(t)
	for _, f := range []fixture{all[0], all[4]} {
		for _, c := range []selectiveCase{{SpaceDB: 20}, {MarkDB: 20, Fade: true}} {
			pcm, _ := selectivePCM(f, wireLevels(t, f), c, m, 0)
			whole := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, true)}
			want := whole.Process(pcm, nil)
			for _, size := range []int{1, f.SampleRate / 50, 0} {
				r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, true)}
				rng := rand.New(rand.NewPCG(43, 89))
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
					t.Fatal("short-evidence partition changed stream state")
				}
			}
			r := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, true)}
			r.Process(pcm[:len(pcm)/2], nil)
			r.Reset()
			fresh := &shortNoiseTrial{newNoise(t, f, "guarded-offset", p, true)}
			if !reflect.DeepEqual(r, fresh) {
				t.Fatal("short-evidence reset retained state")
			}
			if got := r.Process(pcm, nil); !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
				t.Fatal("short-evidence reset changed replay")
			}
		}
	}
}

func BenchmarkNoiseShortEvidence(b *testing.B) {
	p, _ := loadNoise(b)
	for _, f := range fixtures(b) {
		if !strings.Contains(f.ID, "mark-high-stop3") || strings.HasPrefix(f.ID, "usos") {
			continue
		}
		b.Run(f.ID, func(b *testing.B) {
			r := &shortNoiseTrial{newNoise(b, f, "guarded-offset", p, false)}
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

// Ensure the shared prototype still honours validation before guard allocation.
func TestNoiseShortInvalidConfiguration(t *testing.T) {
	p, _ := loadNoise(t)
	if r, err := newNoiseTrial(rtty.Config{}, false, "guarded-offset", p); r != nil || err == nil {
		t.Fatal("accepted invalid configuration")
	}
}
