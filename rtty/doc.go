// SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
// SPDX-License-Identifier: GPL-3.0-only

// Package rtty provides configuration and validation for a streaming RTTY modem.
// Text encoding, character framing, PCM reception, and AFSK generation are not
// implemented yet.
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
