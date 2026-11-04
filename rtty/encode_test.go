// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty_test

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func TestEncodeTextVectors(t *testing.T) {
	data, err := os.ReadFile("../testdata/protocol/text.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Name       string
			Input      string
			Normalized string
			Codes      []uint8
		}
		Invalid []struct {
			Input      string
			ByteOffset int `json:"byte_offset"`
			Rune       rune
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Cases) == 0 || len(vectors.Invalid) == 0 {
		t.Fatal("missing independent text vectors")
	}
	for _, tc := range vectors.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := rtty.EncodeText(tc.Input, rtty.TextOptions{Alphabet: rtty.AlphabetUS})
			if err != nil || got.Text != tc.Normalized || !slices.Equal(got.Codes, tc.Codes) {
				t.Fatalf("EncodeText(%q) = %+v, %v; want %q, %v", tc.Input, got, err, tc.Normalized, tc.Codes)
			}
			if tc.Input == "" && got.Codes != nil {
				t.Fatal("empty input must return the zero EncodedText")
			}
		})
	}
	for _, tc := range vectors.Invalid {
		got, err := rtty.EncodeText(tc.Input, rtty.TextOptions{Alphabet: rtty.AlphabetUS})
		requireEmptyEncoding(t, got)
		requireUnsupportedText(t, err, tc.Rune, tc.ByteOffset)
	}
}

type alphabetReference struct {
	letters [32]string
	figures [32]string
}

func loadAlphabetReference(t testing.TB) alphabetReference {
	t.Helper()
	data, err := os.ReadFile("../testdata/protocol/alphabet-us.csv")
	if err != nil {
		t.Fatal(err)
	}
	rows, err := csv.NewReader(strings.NewReader(string(data))).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatalf("invalid independent alphabet: %v, %d rows", err, len(rows))
	}
	controls := map[string]string{
		"NUL": "\x00", "BEL": "\a", "SPACE": " ", "CR": "\r", "LF": "\n",
		"APOSTROPHE": "'", "QUOTE": "\"", "COMMA": ",", "FIGS": "", "LTRS": "",
	}
	var result alphabetReference
	for i, row := range rows[1:] {
		code, err := strconv.Atoi(row[0])
		if err != nil || code != i {
			t.Fatalf("invalid alphabet code %q", row[0])
		}
		for column, table := range []*[32]string{&result.letters, &result.figures} {
			value := row[column+1]
			if control, ok := controls[value]; ok {
				value = control
			} else if len(value) != 1 {
				t.Fatalf("unknown table entry %q", value)
			}
			table[code] = value
		}
	}
	return result
}

func TestEncodeEveryAlphabetEntry(t *testing.T) {
	ref := loadAlphabetReference(t)
	for code := range 32 {
		for _, figures := range []bool{false, true} {
			text := ref.letters[code]
			if figures {
				text = ref.figures[code]
			}
			if text == "" { // FIGS/LTRS are generated shifts, not text input.
				continue
			}
			want := []uint8{31}
			if figures && ref.letters[code] != ref.figures[code] {
				want = append(want, 27)
			}
			want = append(want, uint8(code))
			// Also exercise ASCII lowercase without relying on the encoder's table.
			for _, input := range []string{text, strings.ToLower(text)} {
				got, err := rtty.EncodeText(input, rtty.TextOptions{Alphabet: rtty.AlphabetUS})
				if err != nil || got.Text != text || !slices.Equal(got.Codes, want) {
					t.Fatalf("code %d, figures=%v, input=%q: %+v, %v; want %v", code, figures, input, got, err, want)
				}
			}
		}
	}

	// Reject all other ASCII bytes, including raw ESC/US, which must not be
	// mistaken for instructions to insert Baudot FIGS/LTRS.
	allowed := ref.allowedASCII()
	for ch := range 128 {
		if allowed[ch] {
			continue
		}
		got, err := rtty.EncodeText("ab\r\n"+string(rune(ch)), rtty.TextOptions{Alphabet: rtty.AlphabetUS})
		requireEmptyEncoding(t, got)
		requireUnsupportedText(t, err, rune(ch), 4)
	}
}

func (ref alphabetReference) allowedASCII() [128]bool {
	var allowed [128]bool
	for _, table := range [][32]string{ref.letters, ref.figures} {
		for _, text := range table {
			if len(text) == 1 {
				ch := text[0]
				allowed[ch] = true
				if ch >= 'A' && ch <= 'Z' {
					allowed[ch+32] = true
				}
			}
		}
	}
	return allowed
}

func TestEncodeShiftAndControlBoundaries(t *testing.T) {
	for _, tc := range []struct {
		input string
		codes []uint8
	}{
		{"1  2", []uint8{31, 27, 23, 4, 4, 27, 19}},
		{"1 A", []uint8{31, 27, 23, 4, 31, 3}},
		{"1\r\n2", []uint8{31, 27, 23, 8, 2, 19}},
		{"1 \r\n\x002", []uint8{31, 27, 23, 4, 8, 2, 0, 27, 19}},
		{"1 \a2", []uint8{31, 27, 23, 4, 27, 5, 19}},
		{"1\x00\a2", []uint8{31, 27, 23, 0, 5, 19}},
		{" \r\n", []uint8{31, 4, 8, 2}},
		{"1 a2", []uint8{31, 27, 23, 4, 31, 3, 27, 19}},
		{"1\x00A", []uint8{31, 27, 23, 0, 31, 3}},
	} {
		got, err := rtty.EncodeText(tc.input, rtty.TextOptions{Alphabet: rtty.AlphabetUS})
		if err != nil || got.Text != strings.ToUpper(tc.input) || !slices.Equal(got.Codes, tc.codes) {
			t.Fatalf("EncodeText(%q) = %+v, %v; want %v", tc.input, got, err, tc.codes)
		}
	}
}

func TestEncodeErrorsAndPrecedence(t *testing.T) {
	for _, alphabet := range []rtty.Alphabet{0, 2, 255} {
		opts := rtty.TextOptions{Alphabet: alphabet}
		requireOptionsError(t, opts.Validate(), "Alphabet")
		for _, text := range []string{"", "A", "+", "\xff"} {
			got, err := rtty.EncodeText(text, opts)
			requireEmptyEncoding(t, got)
			requireOptionsError(t, err, "Alphabet")
		}
	}
	opts := rtty.TextOptions{Alphabet: rtty.AlphabetUS}
	if err := opts.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		input  string
		offset int
	}{
		{"\xff", 0}, {"ab\xc3", 2}, {"a\xc3(", 1}, {"\xed\xa0\x80", 0},
		{"\xf4\x90\x80\x80", 0}, {"\xc0\xaf", 0}, {"\xe2\x82", 0},
		{"a\x80", 1}, {"é\xff", 2}, {"\ufffd\xff", 3},
	} {
		got, err := rtty.EncodeText(tc.input, opts)
		requireEmptyEncoding(t, got)
		var detail *rtty.InvalidUTF8Error
		wrapped := fmt.Errorf("context: %w", err)
		if !errors.Is(wrapped, rtty.ErrInvalidInput) || errors.Is(wrapped, rtty.ErrUnsupportedText) ||
			!errors.As(wrapped, &detail) || detail.ByteOffset != tc.offset {
			t.Fatalf("EncodeText(%q): invalid UTF-8 error %v, want offset %d", tc.input, err, tc.offset)
		}
		if !strings.Contains(err.Error(), strconv.Itoa(tc.offset)) {
			t.Fatalf("error omits byte offset: %v", err)
		}
	}
	for _, ch := range []rune{'é', '“', '\ufffd', '\ufeff', 'ı', 'ſ', 'π', '🙂'} {
		got, err := rtty.EncodeText("abc"+string(ch), opts)
		requireEmptyEncoding(t, got)
		requireUnsupportedText(t, err, ch, 3)
	}
	got, err := rtty.EncodeText(strings.Repeat("a1 ", 1024)+"+", opts)
	requireEmptyEncoding(t, got)
	requireUnsupportedText(t, err, '+', 3072)
}

func requireEmptyEncoding(t *testing.T, got rtty.EncodedText) {
	t.Helper()
	if got.Text != "" || got.Codes != nil {
		t.Fatalf("expected zero EncodedText; got %+v", got)
	}
}

func requireUnsupportedText(t *testing.T, err error, ch rune, offset int) {
	t.Helper()
	var detail *rtty.UnsupportedTextError
	wrapped := fmt.Errorf("context: %w", err)
	if !errors.Is(wrapped, rtty.ErrUnsupportedText) || errors.Is(wrapped, rtty.ErrInvalidInput) ||
		!errors.As(wrapped, &detail) || detail.Rune != ch || detail.ByteOffset != offset {
		t.Fatalf("unsupported text error = %v; want %U at byte %d", err, ch, offset)
	}
	if !strings.Contains(err.Error(), fmt.Sprintf("%U", ch)) || !strings.Contains(err.Error(), strconv.Itoa(offset)) {
		t.Fatalf("error omits rune or byte offset: %v", err)
	}
}

func TestEncodeIndependentCallsAndOwnership(t *testing.T) {
	for i := range 8 {
		t.Run(strconv.Itoa(i), func(t *testing.T) {
			t.Parallel()
			opts := rtty.TextOptions{Alphabet: rtty.AlphabetUS}
			first, err := rtty.EncodeText("a1a", opts)
			if err != nil {
				t.Fatal(err)
			}
			bad, err := rtty.EncodeText("1+", opts)
			requireEmptyEncoding(t, bad)
			requireUnsupportedText(t, err, '+', 1)
			second, err := rtty.EncodeText("a1a", opts)
			if err != nil {
				t.Fatal(err)
			}
			first.Codes[0] = 0
			if second.Text != "A1A" || !slices.Equal(second.Codes, []uint8{31, 3, 27, 23, 31, 3}) {
				t.Fatalf("shared storage or shift state: %+v", second)
			}
		})
	}
}

// This test-only interpreter uses the fixed CSV, never the production tables.
// Its scoring view preserves NUL/BEL exactly as source text (unlike a display).
func (ref alphabetReference) decode(t *testing.T, codes []uint8, usos bool) string {
	t.Helper()
	var text strings.Builder
	figures := false
	for _, code := range codes {
		if code > 31 {
			t.Fatalf("illegal code %d", code)
		}
		switch code {
		case 31:
			figures = false
		case 27:
			figures = true
		default:
			if figures {
				text.WriteString(ref.figures[code])
			} else {
				text.WriteString(ref.letters[code])
			}
			if usos && code == 4 {
				figures = false
			}
		}
	}
	return text.String()
}

func FuzzEncodeText(f *testing.F) {
	ref := loadAlphabetReference(f)
	allowed := ref.allowedASCII()
	for _, text := range []string{"", "a1a", "12 34", "1 A", "1 \r\n\x00\a2", "\x00\a", "cq “", "é\xff", "\ufffd", "\x1b\x1f"} {
		f.Add(text, uint8(rtty.AlphabetUS))
	}
	f.Add("", uint8(0))
	f.Add("A", uint8(255))
	f.Fuzz(func(t *testing.T, text string, alphabet uint8) {
		if len(text) > 65536 {
			t.Skip()
		}
		got, err := rtty.EncodeText(text, rtty.TextOptions{Alphabet: rtty.Alphabet(alphabet)})
		if alphabet != uint8(rtty.AlphabetUS) {
			requireEmptyEncoding(t, got)
			requireOptionsError(t, err, "Alphabet")
			return
		}
		if !utf8.ValidString(text) {
			requireEmptyEncoding(t, got)
			var detail *rtty.InvalidUTF8Error
			if !errors.Is(err, rtty.ErrInvalidInput) || !errors.As(err, &detail) ||
				detail.ByteOffset < 0 || detail.ByteOffset >= len(text) || !utf8.ValidString(text[:detail.ByteOffset]) {
				t.Fatalf("invalid UTF-8 error %v for %q", err, text)
			}
			r, size := utf8.DecodeRuneInString(text[detail.ByteOffset:])
			if r != utf8.RuneError || size != 1 {
				t.Fatalf("UTF-8 error points at a valid rune: %v", err)
			}
			return
		}
		for offset, r := range text {
			if r >= 128 || !allowed[r] {
				requireEmptyEncoding(t, got)
				requireUnsupportedText(t, err, r, offset)
				return
			}
		}
		if err != nil {
			t.Fatalf("rejected supported text %q: %v", text, err)
		}
		want := strings.ToUpper(text) // The independently checked repertoire is ASCII.
		if got.Text != want || len(got.Codes) > 2*len(text)+1 {
			t.Fatalf("invalid normalized text or code count: %+v", got)
		}
		if text == "" {
			requireEmptyEncoding(t, got)
			return
		}
		if len(got.Codes) == 0 || got.Codes[0] != 31 {
			t.Fatal("nonempty encoding must begin with LTRS")
		}
		for _, usos := range []bool{false, true} {
			if decoded := ref.decode(t, got.Codes, usos); decoded != want {
				t.Fatalf("USOS=%v: %q decoded as %q via codes %v", usos, text, decoded, got.Codes)
			}
		}
	})
}
