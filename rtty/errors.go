// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

import "errors"

var (
	// ErrInvalidConfig identifies an unsupported or invalid Config.
	ErrInvalidConfig = errors.New("rtty: invalid configuration")

	// ErrInvalidOptions identifies invalid decoder options.
	ErrInvalidOptions = errors.New("rtty: invalid options")
)

// ConfigError identifies an invalid configuration field. Field uses the Go
// field name, including a dotted path such as "Baud.Numerator". Reason explains
// the constraint; callers should not parse Error or Reason as a stable format.
// Use errors.Is with ErrInvalidConfig and errors.As with *ConfigError.
type ConfigError struct {
	Field  string
	Reason string
}

// Error describes the invalid field and reason.
func (e *ConfigError) Error() string {
	return ErrInvalidConfig.Error() + ": " + e.Field + ": " + e.Reason
}

// Unwrap returns ErrInvalidConfig.
func (e *ConfigError) Unwrap() error { return ErrInvalidConfig }

// OptionsError identifies an invalid option field. Field uses the Go field
// path, such as "Squelch.OpenDB". Reason explains the constraint; callers should
// not parse Error or Reason as a stable format. Use errors.Is with
// ErrInvalidOptions and errors.As with *OptionsError.
type OptionsError struct {
	Field  string
	Reason string
}

// Error describes the invalid field and reason.
func (e *OptionsError) Error() string {
	return ErrInvalidOptions.Error() + ": " + e.Field + ": " + e.Reason
}

// Unwrap returns ErrInvalidOptions.
func (e *OptionsError) Unwrap() error { return ErrInvalidOptions }
