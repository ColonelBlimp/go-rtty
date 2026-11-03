// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

package reference

import (
	"encoding/binary"
	"testing"
)

func sampleWAV() []byte {
	b := make([]byte, 48)
	copy(b, "RIFF")
	binary.LittleEndian.PutUint32(b[4:], 40)
	copy(b[8:], "WAVEfmt ")
	binary.LittleEndian.PutUint32(b[16:], 16)
	binary.LittleEndian.PutUint16(b[20:], 1)
	binary.LittleEndian.PutUint16(b[22:], 1)
	binary.LittleEndian.PutUint32(b[24:], 12000)
	binary.LittleEndian.PutUint32(b[28:], 24000)
	binary.LittleEndian.PutUint16(b[32:], 2)
	binary.LittleEndian.PutUint16(b[34:], 16)
	copy(b[36:], "data")
	binary.LittleEndian.PutUint32(b[40:], 4)
	return b
}

func TestWAVRejectsMalformed(t *testing.T) {
	b := sampleWAV()
	if rate, samples, err := inspectWAV(b); rate != 12000 || samples != 2 || err != nil {
		t.Fatalf("valid WAV: %d, %d, %v", rate, samples, err)
	}
	for n := range len(b) {
		if _, _, err := inspectWAV(b[:n]); err == nil {
			t.Fatalf("accepted truncation at byte %d", n)
		}
	}
	for _, offset := range []int{0, 4, 8, 16, 20, 22, 24, 28, 32, 34, 40} {
		bad := append([]byte(nil), b...)
		bad[offset] = 0xff
		if _, _, err := inspectWAV(bad); err == nil {
			t.Errorf("accepted corrupt header at byte %d", offset)
		}
	}
	// An odd-sized ancillary chunk must include its padding byte.
	padded := append(append([]byte(nil), b[:36]...), []byte{'J', 'U', 'N', 'K', 1, 0, 0, 0, 42, 0}...)
	padded = append(padded, b[36:]...)
	binary.LittleEndian.PutUint32(padded[4:], uint32(len(padded)-8))
	if _, _, err := inspectWAV(padded); err != nil {
		t.Fatal("valid padded chunk:", err)
	}
	duplicate := append(append([]byte(nil), b...), b[36:]...)
	binary.LittleEndian.PutUint32(duplicate[4:], uint32(len(duplicate)-8))
	if _, _, err := inspectWAV(duplicate); err == nil {
		t.Fatal("accepted duplicate data chunk")
	}
	large := append([]byte(nil), b...)
	binary.LittleEndian.PutUint32(large[40:], ^uint32(0))
	if _, _, err := inspectWAV(large); err == nil {
		t.Fatal("accepted oversized data chunk")
	}
}

func FuzzInspectWAV(f *testing.F) {
	f.Add(sampleWAV())
	f.Add([]byte("RIFF"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, b []byte) {
		rate, samples, err := inspectWAV(b)
		if err == nil && ((rate != 12000 && rate != 48000) || samples <= 0 || samples > len(b)/2) {
			t.Fatalf("invalid accepted metadata: %d Hz, %d samples", rate, samples)
		}
	})
}
