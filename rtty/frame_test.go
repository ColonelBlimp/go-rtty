// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty_test

import (
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/ColonelBlimp/go-rtty/rtty"
)

func TestFrameLiteralVectors(t *testing.T) {
	data, err := os.ReadFile("../testdata/protocol/framing.json")
	if err != nil {
		t.Fatal(err)
	}
	var vectors struct {
		Cases []struct {
			Code             uint8
			StopHalfBits     rtty.StopBits `json:"stop_half_bits"`
			Levels           string
			BoundaryHalfBits []uint32 `json:"boundary_half_bits"`
		}
	}
	if err := json.Unmarshal(data, &vectors); err != nil {
		t.Fatal(err)
	}
	if len(vectors.Cases) != 8 {
		t.Fatal("missing independent framing vectors")
	}
	for _, tc := range vectors.Cases {
		t.Run(fmt.Sprintf("code%d/stop%d", tc.Code, tc.StopHalfBits), func(t *testing.T) {
			symbols, err := rtty.FrameCodes([]uint8{tc.Code}, tc.StopHalfBits)
			if err != nil {
				t.Fatal(err)
			}
			var levels strings.Builder
			boundaries := []uint32{0}
			for _, symbol := range symbols {
				level := "0"
				if symbol.Mark {
					level = "1"
				}
				levels.WriteString(strings.Repeat(level, int(symbol.HalfBits)))
				boundaries = append(boundaries, boundaries[len(boundaries)-1]+symbol.HalfBits)
			}
			if levels.String() != tc.Levels || !slices.Equal(boundaries, tc.BoundaryHalfBits) {
				t.Fatalf("got levels %s, boundaries %v; want %s, %v", levels.String(), boundaries, tc.Levels, tc.BoundaryHalfBits)
			}
		})
	}
}

// These expectations use the pre-existing literal wire-bit CSV, independently
// of production framing logic and alphabet tables.
func loadFrameReference(t testing.TB, stop rtty.StopBits) [32][]rtty.Symbol {
	t.Helper()
	file, err := os.Open("../testdata/protocol/alphabet-us.csv")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	rows, err := csv.NewReader(file).ReadAll()
	if err != nil || len(rows) != 33 {
		t.Fatalf("invalid independent wire-bit table: %v", err)
	}
	var frames [32][]rtty.Symbol
	for code, row := range rows[1:] {
		if len(row) != 4 || len(row[3]) != 5 {
			t.Fatalf("invalid wire-bit row %d", code)
		}
		frame := []rtty.Symbol{{HalfBits: 2}}
		for _, bit := range row[3] {
			if bit != '0' && bit != '1' {
				t.Fatal("invalid reference bit")
			}
			frame = append(frame, rtty.Symbol{Mark: bit == '1', HalfBits: 2})
		}
		frames[code] = append(frame, rtty.Symbol{Mark: true, HalfBits: uint32(stop)})
	}
	return frames
}

func TestFrameEveryCodeAndAdjacentPair(t *testing.T) {
	for _, stop := range []rtty.StopBits{rtty.StopBitsOnePointFive, rtty.StopBitsTwo} {
		ref := loadFrameReference(t, stop)
		for first := range 32 {
			for second := range 32 {
				codes := []uint8{uint8(first), uint8(second)}
				got, err := rtty.FrameCodes(codes, stop)
				want := slices.Concat(ref[first], ref[second])
				if err != nil || !slices.Equal(got, want) {
					t.Fatalf("codes %v, stop %d: %v, %v; want %v", codes, stop, got, err, want)
				}
			}
		}
	}
}

func TestFrameErrorsAndEmptyInput(t *testing.T) {
	for stop := range 256 {
		if stop == 3 || stop == 4 {
			continue
		}
		for _, codes := range [][]uint8{nil, {}, {0, 31}, {255}} {
			got, err := rtty.FrameCodes(codes, rtty.StopBits(stop))
			if got != nil {
				t.Fatal("invalid stop returned partial output")
			}
			requireOptionsError(t, err, "StopBits")
		}
	}
	for _, stop := range []rtty.StopBits{rtty.StopBitsOnePointFive, rtty.StopBitsTwo} {
		for _, codes := range [][]uint8{nil, {}} {
			got, err := rtty.FrameCodes(codes, stop)
			if got != nil || err != nil {
				t.Fatalf("empty input: %v, %v", got, err)
			}
		}
		for code := 32; code < 256; code++ {
			got, err := rtty.FrameCodes([]uint8{31, 0, uint8(code), 255}, stop)
			var detail *rtty.InvalidCodeError
			wrapped := fmt.Errorf("context: %w", err)
			if got != nil || !errors.Is(wrapped, rtty.ErrInvalidInput) ||
				!errors.As(wrapped, &detail) || detail.CodeIndex != 2 || detail.Code != uint8(code) {
				t.Fatalf("code %d: %v, %v", code, got, err)
			}
			if !strings.Contains(err.Error(), fmt.Sprintf("code %d", code)) || !strings.Contains(err.Error(), "index 2") {
				t.Fatalf("error omits code or index: %v", err)
			}
		}
	}
}

func TestFramePartitionInvarianceAndOwnership(t *testing.T) {
	for _, stop := range []rtty.StopBits{rtty.StopBitsOnePointFive, rtty.StopBitsTwo} {
		t.Run(fmt.Sprint(stop), func(t *testing.T) {
			t.Parallel()
			codes := []uint8{31, 31, 27, 27, 0, 0}
			for code := range 32 {
				codes = append(codes, uint8(code))
			}
			original := slices.Clone(codes)
			whole, err := rtty.FrameCodes(codes, stop)
			if err != nil {
				t.Fatal(err)
			}
			// Every split includes the empty prefix/suffix and boundaries beside
			// NUL, shifts, and repeated mark/space states.
			for split := 0; split <= len(codes); split++ {
				first, err1 := rtty.FrameCodes(codes[:split], stop)
				second, err2 := rtty.FrameCodes(codes[split:], stop)
				if err1 != nil || err2 != nil || !slices.Equal(slices.Concat(first, second), whole) {
					t.Fatalf("split %d changed framing: %v, %v", split, err1, err2)
				}
			}
			rng := rand.New(rand.NewPCG(42, 7))
			for trial := range 16 {
				var joined []rtty.Symbol
				for i := 0; i < len(codes); {
					n := 1
					if trial != 0 { // First trial frames one code per call.
						n = 1 + rng.IntN(len(codes)-i)
					}
					part, err := rtty.FrameCodes(codes[i:i+n], stop)
					if err != nil {
						t.Fatal(err)
					}
					joined = append(joined, part...)
					i += n
				}
				if !slices.Equal(joined, whole) {
					t.Fatalf("partition %d changed framing", trial)
				}
			}
			if !slices.Equal(codes, original) {
				t.Fatal("input was modified")
			}
			other, err := rtty.FrameCodes(codes, stop)
			if err != nil {
				t.Fatal(err)
			}
			whole[0] = rtty.Symbol{Mark: true, HalfBits: 99}
			if other[0] != (rtty.Symbol{HalfBits: 2}) {
				t.Fatal("calls share output storage")
			}
			codes[0] = 0
			if !other[1].Mark {
				t.Fatal("output aliases input")
			}
		})
	}
}
