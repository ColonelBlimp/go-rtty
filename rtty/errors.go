// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

import (
	"errors"
	"fmt"
)

var (
	// ErrInvalidConfig identifies an unsupported or invalid Config.
	ErrInvalidConfig = errors.New("rtty: invalid configuration")

	// ErrInvalidOptions identifies invalid encoding or decoder options.
	ErrInvalidOptions = errors.New("rtty: invalid options")

	// ErrInvalidInput identifies malformed input, including invalid UTF-8.
	ErrInvalidInput = errors.New("rtty: invalid input")

	// ErrUnsupportedText identifies a valid UTF-8 rune outside the chosen alphabet.
	ErrUnsupportedText = errors.New("rtty: unsupported text")
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

// InvalidUTF8Error locates the first malformed UTF-8 encoding in the original
// input string. ByteOffset is zero-based and is not a rune index. A correctly
// encoded U+FFFD is instead unsupported text in AlphabetUS.
// Use errors.Is with ErrInvalidInput and errors.As with *InvalidUTF8Error.
type InvalidUTF8Error struct {
	ByteOffset int
}

// Error describes the malformed encoding's byte offset.
func (e *InvalidUTF8Error) Error() string {
	return fmt.Sprintf("%s: invalid UTF-8 at byte %d", ErrInvalidInput, e.ByteOffset)
}

// Unwrap returns ErrInvalidInput.
func (e *InvalidUTF8Error) Unwrap() error { return ErrInvalidInput }

// UnsupportedTextError identifies a rune not representable in the selected
// alphabet. Rune is the original input rune; ByteOffset is its zero-based byte
// offset in the original string. Use errors.Is with ErrUnsupportedText and
// errors.As with *UnsupportedTextError.
type UnsupportedTextError struct {
	Rune       rune
	ByteOffset int
}

// Error describes the unsupported rune and its byte offset.
func (e *UnsupportedTextError) Error() string {
	return fmt.Sprintf("%s: %U at byte %d", ErrUnsupportedText, e.Rune, e.ByteOffset)
}

// Unwrap returns ErrUnsupportedText.
func (e *UnsupportedTextError) Unwrap() error { return ErrUnsupportedText }
