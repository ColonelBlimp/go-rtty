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

func ExampleEncodeText() {
	encoded, err := rtty.EncodeText("a1a", rtty.TextOptions{Alphabet: rtty.AlphabetUS})
	if err != nil {
		panic(err)
	}
	fmt.Println(encoded.Text)
	fmt.Println(encoded.Codes)
	// Output:
	// A1A
	// [31 3 27 23 31 3]
}

func ExampleEncodeText_unsupported() {
	_, err := rtty.EncodeText("CQ “", rtty.TextOptions{Alphabet: rtty.AlphabetUS})
	var detail *rtty.UnsupportedTextError
	if errors.Is(err, rtty.ErrUnsupportedText) && errors.As(err, &detail) {
		fmt.Printf("unsupported %U at byte %d\n", detail.Rune, detail.ByteOffset)
	}
	// Output: unsupported U+201C at byte 3
}

func ExampleFrameCodes() {
	symbols, err := rtty.FrameCodes([]uint8{3}, rtty.StopBitsOnePointFive)
	if err != nil {
		panic(err)
	}
	for _, symbol := range symbols {
		fmt.Printf("mark=%v half-bits=%d\n", symbol.Mark, symbol.HalfBits)
	}
	// Output:
	// mark=false half-bits=2
	// mark=true half-bits=2
	// mark=true half-bits=2
	// mark=false half-bits=2
	// mark=false half-bits=2
	// mark=false half-bits=2
	// mark=true half-bits=3
}
