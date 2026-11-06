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
)

func TestSelectiveChannelCalibration(t *testing.T) {
	m, _ := loadSelective(t)
	for _, f := range fixtures(t) {
		identity := m
		identity.Gain = 1
		levels := wireLevels(t, f)
		pcm, clips := selectivePCM(f, levels, selectiveCase{}, identity, 0)
		if clips != 0 || !slices.Equal(pcm, f.PCM) {
			t.Fatal("identity channel changed independent PCM")
		}
		// Independent LTRS starts with space, then five marks and mark stop.
		for i := f.Timing.Lead; i < f.Timing.Lead+f.Timing.Bit; i++ {
			if levels[i] {
				t.Fatal("start interval is not space")
			}
		}
		for i := f.Timing.Lead + f.Timing.Bit; i < f.Timing.Lead+6*f.Timing.Bit+f.Timing.Stop; i++ {
			if !levels[i] {
				t.Fatal("LTRS data/stop interval is not mark")
			}
		}
	}
	for _, rate := range []int{12000, 48000} {
		for _, mark := range []bool{false, true} {
			f := fixture{SampleRate: rate, PCM: make([]int16, rate)}
			levels := make([]bool, rate)
			for i := range f.PCM {
				f.PCM[i] = int16(math.RoundToEven(16000 * math.Sin(2*math.Pi*1500*float64(i)/float64(rate))))
				levels[i] = mark
			}
			c := selectiveCase{SpaceDB: 20}
			if mark {
				c = selectiveCase{MarkDB: 20}
			}
			pcm, clips := selectivePCM(f, levels, c, m, 0)
			var a, b float64
			for i := rate / 20; i < rate; i++ {
				x := float64(f.PCM[i]) * m.Gain
				a += x * x
				y := float64(pcm[i])
				b += y * y
			}
			attenuation := 10 * math.Log10(a/b)
			if clips != 0 || math.Abs(attenuation-20) > .05 {
				t.Fatalf("rate%d mark%v attenuation%g clips%d", rate, mark, attenuation, clips)
			}
		}
	}
}

func TestTrialRatioMatchesCurrent(t *testing.T) {
	for _, f := range fixtures(t) {
		current := newReceiver(t, f.Config, true)
		candidate := trial(t, f, "ratio", true)
		want := current.Process(f.PCM, nil)
		got := candidate.Process(f.PCM, nil)
		if !slices.Equal(got, want) || candidate.Diagnostics != current.Diagnostics || !reflect.DeepEqual(candidate.frame, current.framer) {
			t.Fatalf("%s: ratio trial changed observations/state", f.ID)
		}
	}
}

func TestThresholdStreamingAndReset(t *testing.T) {
	m, _ := loadSelective(t)
	all := fixtures(t)
	for _, f := range []fixture{all[0], all[4]} {
		for _, c := range m.Cases {
			if c.ID != "mark-minus20" && c.ID != "space-fade20" && c.ID != "space-minus12-noise10" {
				continue
			}
			pcm, _ := selectivePCM(f, wireLevels(t, f), c, m, m.Seeds[0])
			for _, mode := range m.Candidates {
				whole := trial(t, f, mode, true)
				want := whole.Process(pcm, nil)
				for _, size := range []int{1, f.SampleRate / 50, 0} {
					r := trial(t, f, mode, true)
					rng := rand.New(rand.NewPCG(37, 73))
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
						t.Fatalf("%s/%s/%s chunk%d changed output/state", f.ID, c.ID, mode, size)
					}
				}
				r := trial(t, f, mode, true)
				r.Process(pcm[:len(pcm)/2], nil)
				if r.peakMark == 0 || r.peakSpace == 0 {
					t.Fatal("test did not train both peak estimates")
				}
				r.Reset()
				fresh := trial(t, f, mode, true)
				if !reflect.DeepEqual(r, fresh) {
					t.Fatal("reset retained candidate history")
				}
				got := r.Process(pcm, nil)
				if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
					t.Fatal("reset changed subsequent stream")
				}
			}
		}
	}
}

func BenchmarkThresholdCandidates(b *testing.B) {
	m, _ := loadSelective(b)
	for _, f := range fixtures(b) {
		if !strings.Contains(f.ID, "mark-high-stop3") || strings.HasPrefix(f.ID, "usos") {
			continue
		}
		for _, mode := range m.Candidates {
			b.Run(f.ID+"/"+mode, func(b *testing.B) {
				r := trial(b, f, mode, false)
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
