// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty_test

import (
	"errors"
	"fmt"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func ExampleConfig_ToneFrequencies() {
	cfg := rtty.DefaultConfig()
	cfg.CenterHz = 2210
	cfg.Polarity = rtty.MarkLow
	mark, space, err := cfg.ToneFrequencies()
	if err != nil {
		panic(err)
	}
	fmt.Printf("mark %.0f Hz, space %.0f Hz\n", mark, space)
	// Output: mark 2125 Hz, space 2295 Hz
}

func ExampleConfig_Validate() {
	cfg := rtty.DefaultConfig()
	cfg.SampleRate = 8000
	err := cfg.Validate()
	var detail *rtty.ConfigError
	if errors.Is(err, rtty.ErrInvalidConfig) && errors.As(err, &detail) {
		fmt.Println("invalid field:", detail.Field)
	}
	// Output: invalid field: SampleRate
}

func ExampleDecoderOptions_Validate() {
	opts := rtty.DefaultDecoderOptions()
	opts.Squelch.Enabled = false
	opts.USOS = true
	fmt.Println(opts.Validate())
	fmt.Println("USOS:", opts.USOS, "squelch:", opts.Squelch.Enabled)
	// Output:
	// <nil>
	// USOS: true squelch: false
}
