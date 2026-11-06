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

func TestTwoPathImpulseAndIdentity(t *testing.T) {
	m, _ := loadPath(t)
	m.Gain = 1
	f := fixture{SampleRate: 12000, PCM: []int16{16384, 0, 0, 0, 0}}
	got, clips := twoPathPCM(f, twoPathCase{Delay: 2, Echo: .5}, m, 0)
	if clips != 0 || !slices.Equal(got, []int16{10923, 0, 5461, 0, 0, 0, 0}) {
		t.Fatalf("causal impulse response %v clips%d", got, clips)
	}
	for _, f := range fixtures(t) {
		got, clips := twoPathPCM(f, twoPathCase{}, m, 0)
		if clips != 0 || !slices.Equal(got, f.PCM) {
			t.Fatal("identity path changed independent audio")
		}
		got[0]++
		if got[0] == f.PCM[0] {
			t.Fatal("channel aliases source storage")
		}
	}
}

func TestTwoPathFrequencyResponse(t *testing.T) {
	m, _ := loadPath(t)
	for _, rate := range []int{12000, 48000} {
		for _, hz := range []float64{1415, 1585} {
			f := fixture{SampleRate: rate, PCM: make([]int16, rate)}
			for i := range f.PCM {
				f.PCM[i] = int16(math.RoundToEven(16000 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate))))
			}
			for _, c := range []twoPathCase{{Delay: 34, Echo: .5}, {Delay: 34, Echo: .95}, {Delay: 38, Echo: .5}, {Delay: 38, Echo: .95}} {
				pcm, clips := twoPathPCM(f, c, m, 0)
				delay := c.Delay * (rate / 12000)
				var input, output float64
				for i := rate / 10; i < rate; i++ {
					a := float64(f.PCM[i]) * m.Gain
					b := float64(pcm[i])
					input += a * a
					output += b * b
				}
				measured := math.Sqrt(output / input)
				predicted := twoPathGain(hz, delay, rate, c.Echo)
				errorDB := 20 * math.Log10(measured/predicted)
				if clips != 0 || len(pcm) != len(f.PCM)+delay || math.Abs(errorDB) > .1 {
					t.Fatalf("rate%d tone%g delay%d echo%g: gain%g predicted%g dBerror%g", rate, hz, delay, c.Echo, measured, predicted, errorDB)
				}
				t.Logf("rate%d tone%g ticks%d echo%g measured_gain_db=%.3f predicted_gain_db=%.3f", rate, hz, c.Delay, c.Echo, 20*math.Log10(measured), 20*math.Log10(predicted))
			}
		}
	}
}

func TestPathStreamingReset(t *testing.T) {
	m, _ := loadPath(t)
	all := fixtures(t)
	for _, f := range []fixture{all[0], all[4]} {
		for _, c := range []twoPathCase{m.Cases[3], m.Cases[5], m.Cases[7]} {
			pcm, _ := twoPathPCM(f, c, m, m.Seeds[0])
			for _, mode := range m.Candidates {
				whole := pathReceiver(t, f, mode, true)
				want := whole.engine.Process(pcm, nil)
				for _, size := range []int{1, f.SampleRate / 50, 0} {
					r := pathReceiver(t, f, mode, true)
					rng := rand.New(rand.NewPCG(47, 97))
					var got []Observation
					for pos := 0; pos < len(pcm); {
						n := size
						if n == 0 {
							n = 1 + rng.IntN(2048)
						}
						n = min(n, len(pcm)-pos)
						got = r.engine.Process(pcm[pos:pos+n], got)
						got = r.engine.Process(nil, got)
						pos += n
					}
					if !slices.Equal(got, want) || !reflect.DeepEqual(r.engine, whole.engine) {
						t.Fatalf("%s/%s/%s chunk%d changed state", f.ID, c.ID, mode, size)
					}
				}
				r := pathReceiver(t, f, mode, true)
				r.engine.Process(pcm[:len(pcm)/2], nil)
				r.engine.Reset()
				fresh := pathReceiver(t, f, mode, true)
				if !reflect.DeepEqual(r.engine, fresh.engine) {
					t.Fatal("path receiver reset retained history")
				}
				got := r.engine.Process(pcm, nil)
				if !slices.Equal(got, want) || !reflect.DeepEqual(r.engine, whole.engine) {
					t.Fatal("path reset changed replay")
				}
				codes, _ := codesAndText(got)
				off := pathReceiver(t, f, mode, false)
				offCodes, _ := codesAndText(off.engine.Process(pcm, nil))
				if !slices.Equal(codes, offCodes) {
					t.Fatal("text policy changed demodulation")
				}
			}
		}
	}
}

func BenchmarkPathCandidates(b *testing.B) {
	m, _ := loadPath(b)
	for _, f := range fixtures(b) {
		if !strings.Contains(f.ID, "mark-high-stop3") || strings.HasPrefix(f.ID, "usos") {
			continue
		}
		for _, mode := range m.Candidates {
			b.Run(f.ID+"/"+mode, func(b *testing.B) {
				r := pathReceiver(b, f, mode, false)
				out := make([]Observation, 0, len(f.ExpectedCodes)+1)
				chunk := f.SampleRate / 50
				b.ReportAllocs()
				b.SetBytes(int64(len(f.PCM) * 2))
				b.ResetTimer()
				for range b.N {
					r.engine.Reset()
					out = out[:0]
					for pos := 0; pos < len(f.PCM); pos += chunk {
						out = r.engine.Process(f.PCM[pos:min(pos+chunk, len(f.PCM))], out)
					}
					if len(out) != len(f.ExpectedCodes) {
						b.Fatal("lost clean frames")
					}
				}
				b.ReportMetric(b.Elapsed().Seconds()/float64(b.N)/(float64(len(f.PCM))/float64(f.SampleRate)), "RTF")
			})
		}
	}
}
