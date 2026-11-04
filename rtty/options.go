// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

// DecoderOptions specifies text unshift policy and candidate squelch settings.
// The zero value explicitly disables both USOS and squelch. Start with
// DefaultDecoderOptions for the intended default receive profile.
type DecoderOptions struct {
	USOS    bool // Unshift to letters on SPACE when true; CR/LF do not unshift.
	Squelch SquelchOptions
}

// SquelchOptions specifies candidate signal-gating thresholds. OpenDB and
// CloseDB refer to a future detector's tone-to-noise metric, not probabilities
// of correctness. These values have not been qualified against an HF corpus.
type SquelchOptions struct {
	Enabled  bool
	OpenDB   float64 // Open threshold; finite even when disabled.
	CloseDB  float64 // Close threshold; finite even when disabled.
	HangBits uint32  // Hang interval in bit periods, not Process calls.
}

// DefaultDecoderOptions returns USOS off and squelch enabled, with 6 dB open,
// 3 dB close, and an eight-bit hang interval. Each call returns an independent
// value. These are candidate defaults pending receiver evaluation.
func DefaultDecoderOptions() DecoderOptions {
	return DecoderOptions{
		Squelch: SquelchOptions{Enabled: true, OpenDB: 6, CloseDB: 3, HangBits: 8},
	}
}

// Validate returns an *OptionsError for the first invalid field. Thresholds must
// always be finite. With squelch enabled, 0 <= CloseDB < OpenDB <= 40 and
// 1 <= HangBits <= 100 are required. With squelch disabled, finite thresholds
// and HangBits are ignored; zero DecoderOptions is valid. No fields are changed.
func (o DecoderOptions) Validate() error {
	s := o.Squelch
	if !finite(s.OpenDB) {
		return &OptionsError{Field: "Squelch.OpenDB", Reason: "must be finite"}
	}
	if !finite(s.CloseDB) {
		return &OptionsError{Field: "Squelch.CloseDB", Reason: "must be finite"}
	}
	if !s.Enabled {
		return nil
	}
	if s.OpenDB <= 0 || s.OpenDB > 40 {
		return &OptionsError{Field: "Squelch.OpenDB", Reason: "must be greater than 0 and at most 40 dB"}
	}
	if s.CloseDB < 0 || s.CloseDB >= s.OpenDB {
		return &OptionsError{Field: "Squelch.CloseDB", Reason: "must be nonnegative and below Squelch.OpenDB"}
	}
	if s.HangBits < 1 || s.HangBits > 100 {
		return &OptionsError{Field: "Squelch.HangBits", Reason: "must be between 1 and 100 bits"}
	}
	return nil
}
