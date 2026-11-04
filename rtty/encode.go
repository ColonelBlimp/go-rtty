// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

import (
	"strings"
	"unicode/utf8"
)

// TextOptions selects the text alphabet. The zero value is invalid; specify
// TextOptions{Alphabet: AlphabetUS} explicitly.
type TextOptions struct {
	Alphabet Alphabet
}

// Validate returns an *OptionsError for an unsupported or zero alphabet.
func (o TextOptions) Validate() error {
	if o.Alphabet != AlphabetUS {
		return &OptionsError{Field: "Alphabet", Reason: "must be AlphabetUS"}
	}
	return nil
}

// EncodedText contains normalized source text and its five-bit code sequence.
// Text preserves SPACE, CR, LF, NUL, and BEL; ASCII lowercase becomes uppercase.
// Codes includes generated LTRS/FIGS shifts. Its storage belongs to the caller
// and is independent of other EncodeText calls. No framing or audio is included.
type EncodedText struct {
	Text  string
	Codes []uint8
}

// EncodeText encodes a complete string using the selected alphabet. Each call
// is independent: nonempty text starts with LTRS, emits shifts as needed, and
// repeats FIGS before the first figures character after SPACE. SPACE itself
// does not change the transmitter's shift state. CR, LF, and NUL preserve it.
// BEL is a figures character and therefore may require FIGS.
//
// Validation precedes output allocation. Invalid options take precedence over
// text errors. Malformed UTF-8 returns *InvalidUTF8Error at its first invalid
// byte, even if an earlier valid rune is unsupported. Otherwise the first
// unsupported rune returns *UnsupportedTextError. No replacement, dropped
// character, transliteration, or line-ending conversion is performed.
//
// Empty valid input returns the zero EncodedText, including nil Codes.
// Every error also returns the zero EncodedText, with no partial output.
func EncodeText(text string, opts TextOptions) (EncodedText, error) {
	if err := opts.Validate(); err != nil {
		return EncodedText{}, err
	}
	if !utf8.ValidString(text) {
		for offset := 0; offset < len(text); {
			r, size := utf8.DecodeRuneInString(text[offset:])
			if r == utf8.RuneError && size == 1 {
				return EncodedText{}, &InvalidUTF8Error{ByteOffset: offset}
			}
			offset += size
		}
	}
	for offset, r := range text {
		if r >= utf8.RuneSelf {
			return EncodedText{}, &UnsupportedTextError{Rune: r, ByteOffset: offset}
		}
		if _, shift := lookupUS(upperASCII(byte(r))); shift == textUnsupported {
			return EncodedText{}, &UnsupportedTextError{Rune: r, ByteOffset: offset}
		}
	}
	if text == "" {
		return EncodedText{}, nil
	}

	var normalized strings.Builder
	normalized.Grow(len(text))
	// The capacity is a starting estimate, not a bound: shift-heavy text needs
	// more codes than input bytes. Avoid a potentially overflowing 2*len(text).
	codes := make([]uint8, 1, len(text))
	codes[0] = codeLTRS
	figures, figuresAfterSpace := false, false
	for i := 0; i < len(text); i++ {
		ch := upperASCII(text[i]) // Validation established ASCII-only input.
		code, shift := lookupUS(ch)
		switch shift {
		case textLetters:
			if figures {
				codes = append(codes, codeLTRS)
			}
			figures, figuresAfterSpace = false, false
		case textFigures:
			if !figures || figuresAfterSpace {
				codes = append(codes, codeFIGS)
			}
			figures, figuresAfterSpace = true, false
		}
		codes = append(codes, code)
		if code == codeSpace {
			figuresAfterSpace = true
		}
		normalized.WriteByte(ch)
	}
	return EncodedText{Text: normalized.String(), Codes: codes}, nil
}

const (
	codeSpace uint8 = 4
	codeFIGS  uint8 = 27
	codeLTRS  uint8 = 31
)

type textShift uint8

const (
	textUnsupported textShift = iota
	textEither
	textLetters
	textFigures
)

func upperASCII(ch byte) byte {
	if ch >= 'a' && ch <= 'z' {
		return ch - ('a' - 'A')
	}
	return ch
}

func lookupUS(ch byte) (uint8, textShift) {
	if i := strings.IndexByte(lettersUS, ch); i >= 0 {
		if figuresUS[i] == ch {
			return uint8(i), textEither
		}
		return uint8(i), textLetters
	}
	if i := strings.IndexByte(figuresUS, ch); i >= 0 {
		return uint8(i), textFigures
	}
	return 0, textUnsupported
}
