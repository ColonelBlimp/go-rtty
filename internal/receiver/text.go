// SPDX-FileCopyrightText: 2012 Dave Freese (W1HKJ)
// SPDX-FileCopyrightText: 2012 Stefan Fendt (DL1SMF)
// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only
//
// US table constants adapted from rtty/alphabet.go and its pinned fldigi
// 4.2.06 source. Original GPL-3.0-or-later notice: ../../rtty/NOTICE.alphabet.
// See docs/PROVENANCE.md. The rendering logic is original project code.

package receiver

const lettersUS = "\x00E\nA SIU\rDRJNFCKTZLWHYPQOBG\x00MXV\x00"
const figuresUS = "\x003\n- \a87\r$4',!:(5\")2#6019?&\x00./;\x00"

func (r *Receiver) render(code uint8) string {
	switch code {
	case 31:
		r.figures = false
		return ""
	case 27:
		r.figures = true
		return ""
	}
	table := lettersUS
	if r.figures {
		table = figuresUS
	}
	ch := table[code]
	if r.usos && code == 4 {
		r.figures = false
	}
	if ch == 0 || ch == '\a' {
		return ""
	}
	return table[code : code+1]
}
