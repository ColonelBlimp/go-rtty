// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package rtty

import "fmt"

// Symbol is a logical mark (one) or space (zero) interval. HalfBits measures
// duration in half-bit periods, independently of baud rate and audio polarity.
type Symbol struct {
	Mark     bool
	HalfBits uint32
}

// FrameCodes frames five-bit codes without inserting shifts, lead-in, or tail.
// Each code produces exactly seven unmerged symbols: a space start, data bits
// zero through four (LSB first), and a mark stop. Start and data symbols last
// two half-bits; stop lasts three or four, as selected by stop.
//
// Invalid stop selections return *OptionsError before inspecting codes, even
// for empty input. The first code above 31 returns *InvalidCodeError. The whole
// input is validated before output allocation. An unrepresentable symbol count
// returns ErrInvalidInput. Errors and empty valid input
// return nil symbols. Output storage belongs to the caller, is independent of
// other calls, and does not alias codes. The input is never modified.
func FrameCodes(codes []uint8, stop StopBits) ([]Symbol, error) {
	if stop != StopBitsOnePointFive && stop != StopBitsTwo {
		return nil, &OptionsError{Field: "StopBits", Reason: "must be StopBitsOnePointFive or StopBitsTwo"}
	}
	for i, code := range codes {
		if code > 31 {
			return nil, &InvalidCodeError{CodeIndex: i, Code: code}
		}
	}
	if len(codes) == 0 {
		return nil, nil
	}
	if len(codes) > int(^uint(0)>>1)/7 {
		return nil, fmt.Errorf("%w: too many codes to frame", ErrInvalidInput)
	}
	symbols := make([]Symbol, 7*len(codes))
	for i, code := range codes {
		frame := symbols[i*7 : (i+1)*7]
		frame[0] = Symbol{HalfBits: 2}
		for bit := range 5 {
			frame[bit+1] = Symbol{Mark: code&(1<<bit) != 0, HalfBits: 2}
		}
		frame[6] = Symbol{Mark: true, HalfBits: uint32(stop)}
	}
	return symbols, nil
}
