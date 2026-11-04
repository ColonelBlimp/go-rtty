// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty_test

import (
	"errors"
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func TestDecoderOptionDefaultsAndZero(t *testing.T) {
	want := rtty.DecoderOptions{Squelch: rtty.SquelchOptions{Enabled: true, OpenDB: 6, CloseDB: 3, HangBits: 8}}
	o := rtty.DefaultDecoderOptions()
	if o != want {
		t.Fatalf("default decoder options = %+v, want %+v", o, want)
	}
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	o.Squelch.Enabled = false
	o.USOS = true
	if err := o.Validate(); err != nil || rtty.DefaultDecoderOptions() != want {
		t.Fatalf("independent options: %v", err)
	}
	var zero rtty.DecoderOptions
	if err := zero.Validate(); err != nil || zero.Squelch.Enabled || zero.USOS {
		t.Fatalf("zero options must remain ungated with USOS off: %+v, %v", zero, err)
	}
}

func TestDecoderOptionLimits(t *testing.T) {
	for _, s := range []rtty.SquelchOptions{
		{Enabled: true, OpenDB: 40, CloseDB: 0, HangBits: 1},
		{Enabled: true, OpenDB: 40, CloseDB: math.Nextafter(40, 0), HangBits: 100},
		{Enabled: true, OpenDB: math.SmallestNonzeroFloat64, CloseDB: 0, HangBits: 1},
		// Disabled squelch ignores finite thresholds and hang interval.
		{OpenDB: -1, CloseDB: 50, HangBits: math.MaxUint32},
	} {
		for _, usos := range []bool{false, true} {
			o := rtty.DecoderOptions{USOS: usos, Squelch: s}
			before := o
			if err := o.Validate(); err != nil || o != before {
				t.Fatalf("valid options %+v: %v; mutated=%v", before, err, o != before)
			}
		}
	}
	for _, tc := range []struct {
		name, field string
		change      func(*rtty.SquelchOptions)
	}{
		{"zero-open", "Squelch.OpenDB", func(s *rtty.SquelchOptions) { s.OpenDB = 0 }},
		{"negative-open", "Squelch.OpenDB", func(s *rtty.SquelchOptions) { s.OpenDB = -1 }},
		{"high-open", "Squelch.OpenDB", func(s *rtty.SquelchOptions) { s.OpenDB = math.Nextafter(40, math.Inf(1)) }},
		{"negative-close", "Squelch.CloseDB", func(s *rtty.SquelchOptions) { s.CloseDB = -math.SmallestNonzeroFloat64 }},
		{"equal-thresholds", "Squelch.CloseDB", func(s *rtty.SquelchOptions) { s.CloseDB = s.OpenDB }},
		{"inverted-thresholds", "Squelch.CloseDB", func(s *rtty.SquelchOptions) { s.CloseDB = 7 }},
		{"zero-hang", "Squelch.HangBits", func(s *rtty.SquelchOptions) { s.HangBits = 0 }},
		{"long-hang", "Squelch.HangBits", func(s *rtty.SquelchOptions) { s.HangBits = 101 }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			o := rtty.DefaultDecoderOptions()
			tc.change(&o.Squelch)
			requireOptionsError(t, o.Validate(), tc.field)
		})
	}
	for _, enabled := range []bool{false, true} {
		for _, value := range []float64{math.NaN(), math.Inf(1), math.Inf(-1)} {
			o := rtty.DefaultDecoderOptions()
			o.Squelch.Enabled, o.Squelch.OpenDB = enabled, value
			requireOptionsError(t, o.Validate(), "Squelch.OpenDB")
			o = rtty.DefaultDecoderOptions()
			o.Squelch.Enabled, o.Squelch.CloseDB = enabled, value
			requireOptionsError(t, o.Validate(), "Squelch.CloseDB")
		}
	}
}

func requireOptionsError(t *testing.T, err error, field string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected invalid option field %s", field)
	}
	wrapped := fmt.Errorf("caller context: %w", err)
	var detail *rtty.OptionsError
	if !errors.Is(wrapped, rtty.ErrInvalidOptions) || errors.Is(wrapped, rtty.ErrInvalidConfig) ||
		!errors.As(wrapped, &detail) || detail.Field != field || detail.Reason == "" ||
		!strings.Contains(err.Error(), detail.Field) || !strings.Contains(err.Error(), detail.Reason) {
		t.Fatalf("invalid error contract for field %s: %v", field, err)
	}
}

func FuzzDecoderOptionsValidation(f *testing.F) {
	f.Add(false, true, 6.0, 3.0, uint32(8))
	f.Add(false, false, 0.0, 0.0, uint32(0))
	f.Add(true, true, 40.0, 0.0, uint32(100))
	f.Add(false, false, math.NaN(), math.Inf(1), uint32(math.MaxUint32))
	f.Fuzz(func(t *testing.T, usos, enabled bool, open, close float64, hang uint32) {
		o := rtty.DecoderOptions{USOS: usos, Squelch: rtty.SquelchOptions{
			Enabled: enabled, OpenDB: open, CloseDB: close, HangBits: hang,
		}}
		err := o.Validate()
		if err != nil {
			var detail *rtty.OptionsError
			if !errors.Is(err, rtty.ErrInvalidOptions) || !errors.As(err, &detail) || detail.Field == "" || detail.Reason == "" {
				t.Fatalf("invalid options lost error detail: %v", err)
			}
			return
		}
		if math.IsNaN(open) || math.IsNaN(close) || math.IsInf(open, 0) || math.IsInf(close, 0) ||
			(enabled && !(0 <= close && close < open && open <= 40 && 1 <= hang && hang <= 100)) {
			t.Fatalf("accepted invalid options: %+v", o)
		}
	})
}
