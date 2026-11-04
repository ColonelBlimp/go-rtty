// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

import "math"

// Polarity assigns logical mark (one) and space (zero) to audio tones.
// The zero value is invalid. Polarity does not select a radio sideband or
// invert a decoded codeword.
type Polarity uint8

const (
	// MarkHigh assigns mark to the higher audio frequency and space to the lower.
	MarkHigh Polarity = 1
	// MarkLow assigns mark to the lower audio frequency and space to the higher.
	MarkLow Polarity = 2
)

// StopBits selects a character's nominal mark stop interval. Its numeric value
// is the number of half-bits. Only StopBitsOnePointFive and StopBitsTwo are valid;
// the zero value is invalid.
type StopBits uint8

const (
	// StopBitsOnePointFive selects a stop interval of three half-bits (1.5 bits).
	StopBitsOnePointFive StopBits = 3
	// StopBitsTwo selects a stop interval of four half-bits (2 bits).
	StopBitsTwo StopBits = 4
)

// Alphabet selects an explicit teleprinter letters/figures table.
// The zero value is invalid.
type Alphabet uint8

const (
	// AlphabetUS selects the US teleprinter table specified in the project design.
	// For example, figures codes 20, 26, and 30 represent '#', '&', and ';'.
	AlphabetUS Alphabet = 1
)

// BaudRate is a rational symbol rate in bits per second. Both terms must be
// nonzero. Config.Validate accepts fractions equivalent to 500/11 (the default,
// approximately 45.454545 baud) or 909/20 (exactly 45.45 baud). It compares reduced
// fractions without modifying the supplied terms or rounding to floating point.
type BaudRate struct {
	Numerator   uint32
	Denominator uint32
}

// Config describes the initial RTTY profile. Config's zero value is invalid;
// begin with DefaultConfig. Validation never replaces or normalizes fields.
type Config struct {
	SampleRate int      // Mono PCM samples per second: 12000 or 48000.
	Baud       BaudRate // Rational bit rate: equivalent to 500/11 or 909/20.
	CenterHz   float64  // Midpoint of the two audio tones, in Hz.
	ShiftHz    float64  // Tone separation, in Hz: 170.
	Polarity   Polarity
	StopBits   StopBits
	Alphabet   Alphabet
}

// DefaultConfig returns 12 kHz, 500/11 baud, 1500 Hz center, 170 Hz shift,
// MarkHigh, StopBitsOnePointFive, and AlphabetUS. Each call returns an independent
// value. The resulting tones are mark 1585 Hz and space 1415 Hz.
func DefaultConfig() Config {
	return Config{
		SampleRate: 12000,
		Baud:       BaudRate{Numerator: 500, Denominator: 11},
		CenterHz:   1500,
		ShiftHz:    170,
		Polarity:   MarkHigh,
		StopBits:   StopBitsOnePointFive,
		Alphabet:   AlphabetUS,
	}
}

// Validate checks the supported profile and returns a *ConfigError for the
// first invalid field. Frequencies must be finite and both tones must lie in
// [300, SampleRate/2 - 300] Hz, including the endpoints, to reserve guard bands
// at DC and Nyquist. Equivalent baud fractions are accepted without mutation.
func (c Config) Validate() error {
	if c.SampleRate != 12000 && c.SampleRate != 48000 {
		return &ConfigError{Field: "SampleRate", Reason: "must be 12000 or 48000 Hz"}
	}
	if c.Baud.Numerator == 0 {
		return &ConfigError{Field: "Baud.Numerator", Reason: "must be nonzero"}
	}
	if c.Baud.Denominator == 0 {
		return &ConfigError{Field: "Baud.Denominator", Reason: "must be nonzero"}
	}
	g := gcd(c.Baud.Numerator, c.Baud.Denominator)
	p, q := c.Baud.Numerator/g, c.Baud.Denominator/g
	if !(p == 500 && q == 11 || p == 909 && q == 20) {
		return &ConfigError{Field: "Baud", Reason: "must equal 500/11 or 909/20 baud"}
	}
	if !finite(c.CenterHz) {
		return &ConfigError{Field: "CenterHz", Reason: "must be finite"}
	}
	if !finite(c.ShiftHz) || c.ShiftHz != 170 {
		return &ConfigError{Field: "ShiftHz", Reason: "must be 170 Hz"}
	}
	low, high := c.CenterHz-c.ShiftHz/2, c.CenterHz+c.ShiftHz/2
	if low < 300 || high > float64(c.SampleRate)/2-300 {
		return &ConfigError{Field: "CenterHz", Reason: "both tones must leave at least 300 Hz to DC and Nyquist"}
	}
	if c.Polarity != MarkHigh && c.Polarity != MarkLow {
		return &ConfigError{Field: "Polarity", Reason: "must be MarkHigh or MarkLow"}
	}
	if c.StopBits != StopBitsOnePointFive && c.StopBits != StopBitsTwo {
		return &ConfigError{Field: "StopBits", Reason: "must be StopBitsOnePointFive or StopBitsTwo"}
	}
	if c.Alphabet != AlphabetUS {
		return &ConfigError{Field: "Alphabet", Reason: "must be AlphabetUS"}
	}
	return nil
}

// ToneFrequencies validates the complete Config and returns the mark and space
// audio frequencies. On validation failure it returns zero frequencies and a
// *ConfigError. It does not infer polarity from a radio's USB/LSB setting.
func (c Config) ToneFrequencies() (markHz, spaceHz float64, err error) {
	if err = c.Validate(); err != nil {
		return 0, 0, err
	}
	low, high := c.CenterHz-c.ShiftHz/2, c.CenterHz+c.ShiftHz/2
	if c.Polarity == MarkHigh {
		return high, low, nil
	}
	return low, high, nil
}

func gcd(a, b uint32) uint32 {
	for b != 0 {
		a, b = b, a%b
	}
	return a
}

func finite(value float64) bool { return !math.IsNaN(value) && !math.IsInf(value, 0) }
