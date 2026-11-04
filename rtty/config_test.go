// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty_test

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func TestDefaultConfig(t *testing.T) {
	want := rtty.Config{
		SampleRate: 12000, Baud: rtty.BaudRate{Numerator: 500, Denominator: 11},
		CenterHz: 1500, ShiftHz: 170, Polarity: rtty.MarkHigh,
		StopBits: rtty.StopBitsOnePointFive, Alphabet: rtty.AlphabetUS,
	}
	c := rtty.DefaultConfig()
	if c != want {
		t.Fatalf("DefaultConfig() = %+v, want %+v", c, want)
	}
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	mark, space, err := c.ToneFrequencies()
	if err != nil || mark != 1585 || space != 1415 {
		t.Fatalf("default tones = %g, %g, %v", mark, space, err)
	}
	c.Baud.Numerator = 1
	c.Polarity = rtty.MarkLow
	if rtty.DefaultConfig() != want {
		t.Fatal("modifying one config changed subsequent defaults")
	}
}

func TestSupportedProfilesAndEquivalentBaud(t *testing.T) {
	// Large unreduced terms catch overflow in implementations using uint32
	// cross-products; equality here must be rational, not rounded float equality.
	rates := []rtty.BaudRate{
		{Numerator: 500, Denominator: 11},
		{Numerator: 909, Denominator: 20},
		{Numerator: 1000, Denominator: 22},
		{Numerator: 1818, Denominator: 40},
		{Numerator: 500 * 8_589_934, Denominator: 11 * 8_589_934},
		{Numerator: 909 * 4_724_936, Denominator: 20 * 4_724_936},
	}
	for _, rate := range []int{12000, 48000} {
		for _, baud := range rates {
			for _, polarity := range []rtty.Polarity{rtty.MarkHigh, rtty.MarkLow} {
				for _, stop := range []rtty.StopBits{rtty.StopBitsOnePointFive, rtty.StopBitsTwo} {
					c := rtty.DefaultConfig()
					c.SampleRate, c.Baud, c.Polarity, c.StopBits = rate, baud, polarity, stop
					before := c
					if err := c.Validate(); err != nil {
						t.Fatalf("supported config %+v: %v", c, err)
					}
					mark, space, err := c.ToneFrequencies()
					wantMark, wantSpace := 1585.0, 1415.0
					if polarity == rtty.MarkLow {
						wantMark, wantSpace = wantSpace, wantMark
					}
					if err != nil || mark != wantMark || space != wantSpace || c != before {
						t.Fatalf("config %+v: tones %g/%g, error %v, mutated=%v", before, mark, space, err, c != before)
					}
				}
			}
		}
	}
}

func TestConfigPassbandBoundaries(t *testing.T) {
	for _, tc := range []struct {
		rate                int
		center, mark, space float64
	}{
		{12000, 385, 470, 300},
		{12000, 5615, 5700, 5530},
		{48000, 385, 470, 300},
		{48000, 23615, 23700, 23530},
		{12000, 1500.25, 1585.25, 1415.25},
	} {
		c := rtty.DefaultConfig()
		c.SampleRate, c.CenterHz = tc.rate, tc.center
		mark, space, err := c.ToneFrequencies()
		if err != nil || mark != tc.mark || space != tc.space {
			t.Fatalf("%d Hz, center %g: tones %g/%g, error %v", tc.rate, tc.center, mark, space, err)
		}
	}
	for _, rate := range []int{12000, 48000} {
		for _, center := range []float64{
			math.Nextafter(385, math.Inf(-1)),
			math.Nextafter(float64(rate)/2-385, math.Inf(1)),
		} {
			c := rtty.DefaultConfig()
			c.SampleRate, c.CenterHz = rate, center
			requireConfigError(t, c.Validate(), "CenterHz")
		}
	}
}

func TestConfigRejectsInvalidFields(t *testing.T) {
	for _, tc := range []struct {
		name, field string
		change      func(*rtty.Config)
	}{
		{"zero", "SampleRate", func(c *rtty.Config) { *c = rtty.Config{} }},
		{"negative-rate", "SampleRate", func(c *rtty.Config) { c.SampleRate = -12000 }},
		{"8k-rate", "SampleRate", func(c *rtty.Config) { c.SampleRate = 8000 }},
		{"44k-rate", "SampleRate", func(c *rtty.Config) { c.SampleRate = 44100 }},
		{"zero-numerator", "Baud.Numerator", func(c *rtty.Config) { c.Baud.Numerator = 0 }},
		{"zero-denominator", "Baud.Denominator", func(c *rtty.Config) { c.Baud.Denominator = 0 }},
		{"45-baud", "Baud", func(c *rtty.Config) { c.Baud = rtty.BaudRate{Numerator: 45, Denominator: 1} }},
		{"50-baud", "Baud", func(c *rtty.Config) { c.Baud = rtty.BaudRate{Numerator: 50, Denominator: 1} }},
		{"75-baud", "Baud", func(c *rtty.Config) { c.Baud = rtty.BaudRate{Numerator: 75, Denominator: 1} }},
		{"near-preset", "Baud", func(c *rtty.Config) { c.Baud = rtty.BaudRate{Numerator: 4294967001, Denominator: 94489274} }},
		{"max-baud-terms", "Baud", func(c *rtty.Config) { c.Baud = rtty.BaudRate{Numerator: math.MaxUint32, Denominator: math.MaxUint32} }},
		{"negative-center", "CenterHz", func(c *rtty.Config) { c.CenterHz = -1500 }},
		{"zero-center", "CenterHz", func(c *rtty.Config) { c.CenterHz = 0 }},
		{"huge-center", "CenterHz", func(c *rtty.Config) { c.CenterHz = math.MaxFloat64 }},
		{"zero-shift", "ShiftHz", func(c *rtty.Config) { c.ShiftHz = 0 }},
		{"negative-shift", "ShiftHz", func(c *rtty.Config) { c.ShiftHz = -170 }},
		{"425-shift", "ShiftHz", func(c *rtty.Config) { c.ShiftHz = 425 }},
		{"850-shift", "ShiftHz", func(c *rtty.Config) { c.ShiftHz = 850 }},
		{"near-shift", "ShiftHz", func(c *rtty.Config) { c.ShiftHz = math.Nextafter(170, math.Inf(1)) }},
		{"zero-polarity", "Polarity", func(c *rtty.Config) { c.Polarity = 0 }},
		{"unknown-polarity", "Polarity", func(c *rtty.Config) { c.Polarity = 255 }},
		{"zero-stop", "StopBits", func(c *rtty.Config) { c.StopBits = 0 }},
		{"one-stop", "StopBits", func(c *rtty.Config) { c.StopBits = 2 }},
		{"unknown-stop", "StopBits", func(c *rtty.Config) { c.StopBits = 255 }},
		{"zero-alphabet", "Alphabet", func(c *rtty.Config) { c.Alphabet = 0 }},
		{"unknown-alphabet", "Alphabet", func(c *rtty.Config) { c.Alphabet = 255 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := rtty.DefaultConfig()
			tc.change(&c)
			requireConfigError(t, c.Validate(), tc.field)
			mark, space, err := c.ToneFrequencies()
			requireConfigError(t, err, tc.field)
			if mark != 0 || space != 0 {
				t.Fatalf("invalid config returned tones %g/%g", mark, space)
			}
		})
	}
	for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
		c := rtty.DefaultConfig()
		c.CenterHz = value
		requireConfigError(t, c.Validate(), "CenterHz")
		c = rtty.DefaultConfig()
		c.ShiftHz = value
		requireConfigError(t, c.Validate(), "ShiftHz")
	}
}

func requireConfigError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected invalid configuration field %s", field)
	}
	wrapped := fmt.Errorf("caller context: %w", err)
	var detail *rtty.ConfigError
	if !errors.Is(wrapped, rtty.ErrInvalidConfig) || errors.Is(wrapped, rtty.ErrInvalidOptions) ||
		!errors.As(wrapped, &detail) || detail.Field != field || detail.Reason == "" {
		t.Fatalf("invalid error contract for field %s: %v", field, err)
	}
	if !strings.Contains(err.Error(), detail.Field) || !strings.Contains(err.Error(), detail.Reason) {
		t.Fatalf("error omits field or reason: %v", err)
	}
}

func TestIndependentFixtureConfigurations(t *testing.T) {
	// Consume the existing independent transmitter metadata without regenerating
	// fixtures or deriving expected tones from the implementation under test.
	data, err := os.ReadFile("../testdata/manifests/independent.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Fixtures []struct {
			ID              string `json:"id"`
			SampleRate      int    `json:"sample_rate"`
			BaudNumerator   uint32 `json:"baud_numerator"`
			BaudDenominator uint32 `json:"baud_denominator"`
			MarkHz          int    `json:"mark_hz"`
			SpaceHz         int    `json:"space_hz"`
			StopHalfBits    uint8  `json:"stop_half_bits"`
			Alphabet        string `json:"alphabet"`
		} `json:"fixtures"`
	}
	if err := json.Unmarshal(data, &manifest); err != nil {
		t.Fatal(err)
	}
	if len(manifest.Fixtures) == 0 {
		t.Fatal("no independent configurations")
	}
	for _, f := range manifest.Fixtures {
		t.Run(f.ID, func(t *testing.T) {
			if f.Alphabet != "US" {
				t.Fatal("fixture needs an explicit supported alphabet mapping")
			}
			c := rtty.DefaultConfig()
			c.SampleRate = f.SampleRate
			c.Baud = rtty.BaudRate{Numerator: f.BaudNumerator, Denominator: f.BaudDenominator}
			c.CenterHz = float64(f.MarkHz+f.SpaceHz) / 2
			c.ShiftHz = math.Abs(float64(f.MarkHz - f.SpaceHz))
			c.StopBits = rtty.StopBits(f.StopHalfBits)
			if f.MarkHz < f.SpaceHz {
				c.Polarity = rtty.MarkLow
			}
			mark, space, err := c.ToneFrequencies()
			if err != nil || mark != float64(f.MarkHz) || space != float64(f.SpaceHz) {
				t.Fatalf("independent tones: %g/%g, error %v", mark, space, err)
			}
		})
	}
}

func FuzzConfigValidation(f *testing.F) {
	f.Add(12000, uint32(500), uint32(11), 1500.0, 170.0, uint8(1), uint8(3), uint8(1))
	f.Add(48000, uint32(909), uint32(20), 23615.0, 170.0, uint8(2), uint8(4), uint8(1))
	f.Add(12000, uint32(4294967000), uint32(94489274), 385.0, 170.0, uint8(1), uint8(3), uint8(1))
	f.Add(0, uint32(0), uint32(0), math.NaN(), math.Inf(1), uint8(0), uint8(0), uint8(0))
	f.Fuzz(func(t *testing.T, rate int, numerator, denominator uint32, center, shift float64, polarity, stop, alphabet uint8) {
		c := rtty.Config{SampleRate: rate, Baud: rtty.BaudRate{Numerator: numerator, Denominator: denominator},
			CenterHz: center, ShiftHz: shift, Polarity: rtty.Polarity(polarity),
			StopBits: rtty.StopBits(stop), Alphabet: rtty.Alphabet(alphabet)}
		err := c.Validate()
		mark, space, toneErr := c.ToneFrequencies()
		if err != nil {
			var detail *rtty.ConfigError
			if !errors.As(err, &detail) || detail.Field == "" || detail.Reason == "" ||
				!errors.Is(err, rtty.ErrInvalidConfig) || !errors.Is(toneErr, rtty.ErrInvalidConfig) || mark != 0 || space != 0 {
				t.Fatalf("invalid config leaked tones or lost error detail: %v, %v, %g/%g", err, toneErr, mark, space)
			}
			return
		}
		p, q := uint64(numerator), uint64(denominator)
		if numerator == 0 || denominator == 0 || !(p*11 == q*500 || p*20 == q*909) ||
			(rate != 12000 && rate != 48000) || shift != 170 || alphabet != 1 ||
			(polarity != 1 && polarity != 2) || (stop != 3 && stop != 4) {
			t.Fatalf("accepted unsupported config: %+v", c)
		}
		if toneErr != nil || math.IsNaN(mark) || math.IsNaN(space) || mark < 300 || space < 300 ||
			mark > float64(rate)/2-300 || space > float64(rate)/2-300 || math.Abs(math.Abs(mark-space)-170) > 1e-9 ||
			math.Abs((mark+space)/2-center) > 1e-9 || (mark > space) != (c.Polarity == rtty.MarkHigh) {
			t.Fatalf("accepted config has invalid tones: %+v, %g/%g, %v", c, mark, space, toneErr)
		}
	})
}
