// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package receiver

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"math"
	"math/rand/v2"
	"os"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/internal/reference"
	"github.com/ColonelBlimp/go-rtty/rtty"
)

type fixture struct {
	ID              string
	Path            string
	SHA256          string
	SampleRate      int           `json:"sample_rate"`
	MarkHz          float64       `json:"mark_hz"`
	SpaceHz         float64       `json:"space_hz"`
	StopHalfBits    rtty.StopBits `json:"stop_half_bits"`
	BaudNumerator   uint32        `json:"baud_numerator"`
	BaudDenominator uint32        `json:"baud_denominator"`
	ExpectedCodes   []uint8       `json:"expected_codes"`
	ExpectedOn      string        `json:"expected_text_usos_on"`
	ExpectedOff     string        `json:"expected_text_usos_off"`
	Timing          struct {
		Lead int `json:"lead_samples"`
		Bit  int `json:"bit_samples"`
		Stop int `json:"stop_samples"`
	}
	PCM    []int16
	Config rtty.Config
}

func fixtures(t testing.TB) []fixture {
	t.Helper()
	data, err := os.ReadFile("../../testdata/manifests/independent.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct{ Fixtures []fixture }
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Fixtures) != 9 {
		t.Fatal("missing independent fixtures")
	}
	for i := range manifest.Fixtures {
		f := &manifest.Fixtures[i]
		data, err := os.ReadFile("../../testdata/" + f.Path)
		if err != nil {
			t.Fatal(err)
		}
		if fmt.Sprintf("%x", sha256.Sum256(data)) != f.SHA256 {
			t.Fatal("fixture checksum mismatch")
		}
		rate, pcm, err := reference.DecodeWAV(data)
		if err != nil || rate != f.SampleRate {
			t.Fatalf("invalid fixture %s: %v", f.ID, err)
		}
		f.PCM = pcm
		cfg := rtty.DefaultConfig()
		cfg.SampleRate, cfg.CenterHz, cfg.ShiftHz = rate, (f.MarkHz+f.SpaceHz)/2, math.Abs(f.MarkHz-f.SpaceHz)
		cfg.Polarity = rtty.MarkHigh
		if f.MarkHz < f.SpaceHz {
			cfg.Polarity = rtty.MarkLow
		}
		cfg.Baud = rtty.BaudRate{Numerator: f.BaudNumerator, Denominator: f.BaudDenominator}
		cfg.StopBits = f.StopHalfBits
		f.Config = cfg
	}
	return manifest.Fixtures
}

func newReceiver(t testing.TB, cfg rtty.Config, usos bool) *Receiver {
	t.Helper()
	r, err := New(cfg, usos)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func codesAndText(obs []Observation) ([]uint8, string) {
	var codes []uint8
	var text strings.Builder
	for _, o := range obs {
		codes = append(codes, o.Code)
		text.WriteString(o.Text)
	}
	return codes, text.String()
}

func TestIndependentBaseline(t *testing.T) {
	for _, f := range fixtures(t) {
		for _, usos := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/usos%v", f.ID, usos), func(t *testing.T) {
				r := newReceiver(t, f.Config, usos)
				obs := r.Process(f.PCM, nil)
				codes, text := codesAndText(obs)
				want := f.ExpectedOff
				if usos {
					want = f.ExpectedOn
				}
				if !slices.Equal(codes, f.ExpectedCodes) || text != want {
					t.Fatalf("codes %v want %v; text %q want %q; diagnostics %+v", codes, f.ExpectedCodes, text, want, r.Diagnostics)
				}
				var maxDelay, maxStartError, maxTrueDelay float64
				for i, o := range obs {
					if o.Start < 0 || o.End <= o.Start || float64(o.Available) < o.End ||
						(i > 0 && (o.Start < obs[i-1].End || o.Available <= obs[i-1].Available)) {
						t.Fatalf("invalid sample positions: %+v", obs)
					}
					maxDelay = math.Max(maxDelay, float64(o.Available)-o.End)
					trueStart := float64(f.Timing.Lead + i*(6*f.Timing.Bit+f.Timing.Stop))
					trueEnd := trueStart + float64(6*f.Timing.Bit+f.Timing.Stop)
					maxStartError = math.Max(maxStartError, math.Abs(o.Start-trueStart))
					maxTrueDelay = math.Max(maxTrueDelay, float64(o.Available)-trueEnd)
				}
				if r.Diagnostics.Rejected != 0 {
					t.Fatalf("unexpected rejections: %+v", r.Diagnostics)
				}
				t.Logf("samples=%d codes=%d text_bytes=%d code_errors=0 text_errors=0 max_estimated_delay_ms=%.3f max_actual_delay_ms=%.3f max_start_error_ms=%.3f", len(f.PCM), len(codes), len(text), maxDelay*1000/float64(f.SampleRate), maxTrueDelay*1000/float64(f.SampleRate), maxStartError*1000/float64(f.SampleRate))
			})
		}
	}
}

func TestEverySampleSplit(t *testing.T) {
	f := fixtures(t)[0]
	// A short, real prefix contains the first frame plus the following start;
	// no synthetic tail is appended. Exercise every possible PCM split.
	pcm := f.PCM[:11*f.Timing.Bit]
	whole := newReceiver(t, f.Config, false)
	want := whole.Process(pcm, nil)
	if len(want) != 1 {
		t.Fatalf("prefix must contain one complete observation, got %v", want)
	}
	r := newReceiver(t, f.Config, false)
	buf := make([]Observation, 0, 2)
	for split := 0; split <= len(pcm); split++ {
		r.Reset()
		got := r.Process(pcm[:split], buf[:0])
		got = r.Process(pcm[split:], got)
		if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
			t.Fatalf("PCM split %d changed output/state", split)
		}
	}
}

func TestSilenceAndSteadyTones(t *testing.T) {
	for _, rate := range []int{12000, 48000} {
		for _, polarity := range []rtty.Polarity{rtty.MarkHigh, rtty.MarkLow} {
			cfg := rtty.DefaultConfig()
			cfg.SampleRate, cfg.Polarity = rate, polarity
			mark, space, _ := cfg.ToneFrequencies()
			for _, hz := range []float64{0, mark, space} {
				pcm := make([]int16, rate)
				for i := range pcm {
					if hz != 0 {
						pcm[i] = int16(16000 * math.Sin(2*math.Pi*hz*float64(i)/float64(rate)))
					}
				}
				r := newReceiver(t, cfg, false)
				if got := r.Process(pcm, nil); len(got) != 0 {
					t.Fatalf("rate%d polarity%d tone%g produced %v", rate, polarity, hz, got)
				}
			}
		}
	}
}

func TestIncompleteFrameAndEmptyCalls(t *testing.T) {
	f := fixtures(t)[0]
	r := newReceiver(t, f.Config, false)
	// End before the first stop begins: no fabricated final character.
	cut := f.Timing.Lead + 6*f.Timing.Bit
	if got := r.Process(f.PCM[:cut], nil); len(got) != 0 {
		t.Fatalf("premature observation: %v", got)
	}
	clone := newReceiver(t, f.Config, false)
	clone.Process(f.PCM[:cut], nil)
	for range 10 {
		if got := r.Process(nil, nil); got != nil {
			t.Fatal("empty input emitted data")
		}
	}
	if !reflect.DeepEqual(r, clone) {
		t.Fatal("empty input changed state")
	}
	got := r.Process(f.PCM[cut:], nil)
	codes, _ := codesAndText(got)
	if !slices.Equal(codes, f.ExpectedCodes) {
		t.Fatal("resume lost pending frame")
	}
}

func TestInvalidConfig(t *testing.T) {
	if r, err := New(rtty.Config{}, false); r != nil || err == nil {
		t.Fatal("invalid config accepted")
	}
}

func TestDefaultBaudOnIndependentAudio(t *testing.T) {
	// Independent audio uses 909/20; verify that the default 500/11 receive
	// clock also handles this small, explicitly identified baud mismatch.
	for _, f := range fixtures(t) {
		cfg := f.Config
		cfg.Baud = rtty.DefaultConfig().Baud
		r := newReceiver(t, cfg, false)
		codes, text := codesAndText(r.Process(f.PCM, nil))
		if !slices.Equal(codes, f.ExpectedCodes) || text != f.ExpectedOff {
			t.Fatalf("default receive baud failed on %s: %v %q", f.ID, codes, text)
		}
	}
}

func FuzzBoundedPCM(f *testing.F) {
	f.Add([]byte{0, 0, 0, 0}, false)
	f.Add([]byte{0xff, 0x7f, 0, 0x80}, true)
	valid := fixtures(f)[0].PCM[:4096]
	seed := make([]byte, len(valid)*2)
	for i, s := range valid {
		seed[2*i], seed[2*i+1] = byte(s), byte(uint16(s)>>8)
	}
	f.Add(seed, false)
	f.Fuzz(func(t *testing.T, data []byte, highRate bool) {
		if len(data) > 8192 {
			t.Skip()
		}
		pcm := make([]int16, len(data)/2)
		for i := range pcm {
			pcm[i] = int16(uint16(data[2*i]) | uint16(data[2*i+1])<<8)
		}
		cfg := rtty.DefaultConfig()
		if highRate {
			cfg.SampleRate = 48000
		}
		a, b := newReceiver(t, cfg, false), newReceiver(t, cfg, false)
		want := a.Process(pcm, nil)
		var got []Observation
		for _, sample := range pcm {
			got = b.Process([]int16{sample}, got)
		}
		if !slices.Equal(got, want) || !reflect.DeepEqual(a, b) {
			t.Fatal("partition changed state")
		}
		for i, o := range got {
			if o.Code > 31 || o.Start >= o.End || (i > 0 && o.Start < got[i-1].End) {
				t.Fatal("invalid observation")
			}
		}
	})
}

func TestIndependentChunkInvariance(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.ID, func(t *testing.T) {
			t.Parallel()
			whole := newReceiver(t, f.Config, false)
			want := whole.Process(f.PCM, nil)
			for _, size := range []int{1, 127, f.SampleRate / 50, 1024, 0} {
				r := newReceiver(t, f.Config, false)
				rng := rand.New(rand.NewPCG(11, 29))
				var got []Observation
				for pos := 0; pos < len(f.PCM); {
					n := size
					if n == 0 {
						n = 1 + rng.IntN(2048)
					}
					n = min(n, len(f.PCM)-pos)
					got = r.Process(nil, got)
					got = r.Process(f.PCM[pos:pos+n], got)
					pos += n
				}
				if !slices.Equal(got, want) || !reflect.DeepEqual(r, whole) {
					t.Fatalf("chunk size %d changed observations, counters, or final state", size)
				}
			}
		})
	}
}

func TestResetAndIsolation(t *testing.T) {
	for _, f := range fixtures(t) {
		t.Run(f.ID, func(t *testing.T) {
			t.Parallel()
			fresh := newReceiver(t, f.Config, true)
			want := fresh.Process(f.PCM, nil)
			r := newReceiver(t, f.Config, true)
			for _, pos := range []int{1, len(f.PCM) / 7, len(f.PCM) / 2, len(f.PCM) - 1} {
				r.Process(f.PCM[:pos], nil)
				r.Reset()
				got := r.Process(f.PCM, nil)
				if !slices.Equal(got, want) || !reflect.DeepEqual(r, fresh) {
					t.Fatal("reset did not restore stream")
				}
			}
		})
	}
}

func BenchmarkReceiver(b *testing.B) {
	for _, f := range fixtures(b) {
		if !strings.Contains(f.ID, "mark-high-stop3") || strings.HasPrefix(f.ID, "usos") {
			continue
		}
		b.Run(f.ID, func(b *testing.B) {
			r := newReceiver(b, f.Config, false)
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
					b.Fatal("lost observations")
				}
			}
			b.ReportMetric(b.Elapsed().Seconds()/float64(b.N)/(float64(len(f.PCM))/float64(f.SampleRate)), "RTF")
		})
	}
}
