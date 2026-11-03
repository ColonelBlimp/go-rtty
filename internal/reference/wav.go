// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package reference

import (
	"encoding/binary"
	"fmt"
)

// inspectWAV accepts the deliberately narrow fixture format: RIFF/WAVE, mono,
// PCM16, 12 or 48 kHz. It inspects bounded input without allocating PCM storage.
func inspectWAV(b []byte) (rate, samples int, err error) {
	bad := func() (int, int, error) { return 0, 0, fmt.Errorf("invalid fixture WAV") }
	if len(b) < 12 || string(b[:4]) != "RIFF" || string(b[8:12]) != "WAVE" ||
		uint64(binary.LittleEndian.Uint32(b[4:8]))+8 != uint64(len(b)) {
		return bad()
	}
	var haveFormat, haveData bool
	for pos := 12; pos < len(b); {
		if len(b)-pos < 8 {
			return bad()
		}
		kind := string(b[pos : pos+4])
		n := uint64(binary.LittleEndian.Uint32(b[pos+4 : pos+8]))
		pos += 8
		padded := n + n%2
		if padded > uint64(len(b)-pos) {
			return bad()
		}
		chunk := b[pos : pos+int(n)]
		switch kind {
		case "fmt ":
			if haveFormat || haveData || len(chunk) != 16 ||
				binary.LittleEndian.Uint16(chunk[0:2]) != 1 ||
				binary.LittleEndian.Uint16(chunk[2:4]) != 1 ||
				binary.LittleEndian.Uint16(chunk[12:14]) != 2 ||
				binary.LittleEndian.Uint16(chunk[14:16]) != 16 {
				return bad()
			}
			rate = int(binary.LittleEndian.Uint32(chunk[4:8]))
			if (rate != 12000 && rate != 48000) || binary.LittleEndian.Uint32(chunk[8:12]) != uint32(rate*2) {
				return bad()
			}
			haveFormat = true
		case "data":
			if !haveFormat || haveData || n == 0 || n%2 != 0 {
				return bad()
			}
			samples = int(n / 2)
			haveData = true
		}
		pos += int(padded)
	}
	if !haveFormat || !haveData {
		return bad()
	}
	return rate, samples, nil
}
