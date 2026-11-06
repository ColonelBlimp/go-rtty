// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

// Package rtty provides US teleprinter text encoding, framing, and configuration
// for a streaming RTTY modem. PCM reception and AFSK generation
// are not implemented yet.
//
// EncodeText converts valid UTF-8 text to five-bit codes with explicit
// TextOptions{Alphabet: AlphabetUS}. It uppercases ASCII letters, preserves
// supported controls, and rejects unsupported characters without partial output.
// Each nonempty call begins an independent transmission with a letters shift.
// FrameCodes converts codes to canonical mark/space symbols with half-bit
// durations, without inserting shifts or burst padding. It preserves seven
// separate symbols per code, independently of audio polarity or sample rate.
//
// Start with DefaultConfig and DefaultDecoderOptions, then set the caller's
// sample rate, tuning, and audio polarity. Config.Validate accepts the initial
// profile: 12 or 48 kHz PCM, 500/11 or 909/20 baud, 170 Hz shift, US teleprinter
// alphabet, and 1.5 or 2 stop bits. These configuration checks do not establish
// reception or transmit performance.
//
// All frequencies describe audio. Applications are responsible for sideband
// selection, audio devices, radio control, PTT, and logging. Ordinary five-bit
// RTTY has no CRC or forward error correction.
package rtty
