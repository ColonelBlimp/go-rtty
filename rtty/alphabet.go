// SPDX-FileCopyrightText: 2012 Dave Freese (W1HKJ)
// SPDX-FileCopyrightText: 2012 Stefan Fendt (DL1SMF)
// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only
//
// The US tables are adapted from fldigi 4.2.06, src/rtty/rtty.cxx,
// through testdata/protocol/alphabet-us.csv. Controls use ASCII bytes and
// shift entries use non-text placeholders. See NOTICE.alphabet and
// docs/PROVENANCE.md for the original GPL-3.0-or-later notice and source pin.

package rtty

// Each byte position is its five-bit code. FIGS (27) and LTRS (31) use NUL
// placeholders: they change interpretation and have no input text character.
// The actual NUL at index zero is always the first match during encoding.
const (
	lettersUS = "\x00E\nA SIU" + // 0..7
		"\rDRJNFCK" + // 8..15
		"TZLWHYPQ" + // 16..23
		"OBG\x00MXV\x00" // 24..31

	figuresUS = "\x003\n- \a87" + // 0..7
		"\r$4',!:(" + // 8..15
		"5\")2#601" + // 16..23
		"9?&\x00./;\x00" // 24..31
)
