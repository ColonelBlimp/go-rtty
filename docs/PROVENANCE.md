<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Provenance

The repository currently contains configuration/validation APIs, finite text
encoding, development infrastructure, and fixtures. The US mapping tables are
adapted from the source identified below. No receive or transmit DSP has been
copied or ported.

| Material | Source and immutable identity | Treatment |
| --- | --- | --- |
| `rtty/config.go`, `rtty/options.go`, `rtty/errors.go` and accompanying GoDoc/tests | Original implementation of the project's configuration and validation contracts | GPL-3.0-only. No upstream implementation copied or ported. These files define profile identifiers and validation, without alphabet tables or modem DSP. Tests also read the existing independent fixture metadata for tone/profile expectations. |
| `rtty/alphabet.go` | fldigi 4.2.06, `src/rtty/rtty.cxx`, via the pinned `testdata/protocol/alphabet-us.csv`; source archive SHA-256 `4360de225d6f8cfeb9bb6a5be6ac492e52702c8f704f87609c452d3bcb1b6c08` | Tables adapted to immutable Go string constants. Named controls become ASCII bytes; FIGS/LTRS use non-text placeholders. Original copyright and GPL-3.0-or-later notice retained in [rtty/NOTICE.alphabet](../rtty/NOTICE.alphabet); the adapted file is distributed under GPL version 3, with original additions GPL-3.0-only. This is source-derived table data. |
| `rtty/encode.go` and encoder/input error implementations and tests | Original implementation of the project's strict text/shift contract, checked against the pre-existing literal text vectors and CSV | GPL-3.0-only. No upstream encoder logic copied. Tests interpret the fixed CSV independently of production tables, with both USOS policies, in addition to exact golden code comparisons. |
| `testdata/protocol/alphabet-us.csv` | fldigi 4.2.06, `src/rtty/rtty.cxx`, source archive SHA-256 `4360de225d6f8cfeb9bb6a5be6ac492e52702c8f704f87609c452d3bcb1b6c08` | US table transcribed into CSV; controls named explicitly; LSB-first bit strings added. Original notice retained in `testdata/protocol/NOTICE`. GPL-3.0-or-later. |
| Protocol framing/text vectors | Project design and manually specified examples | Original GPL-3.0-only fixtures, specified before production encoding code. Alphabet facts cross-checked against the pinned fldigi and minimodem sources. This is not a claim of external human review. |
| `testdata/independent/*.wav` | Unmodified minimodem 0.24 Debian `0.24-1+b2` binary; exact executable hash in `testdata/manifests/tools.json` | Generated from original project text, no RF recording or channel impairment. Audio is original fixture material under GPL-3.0-only. Commands, timing, and captures are in the manifest. |
| Minimodem behavior comparison | Upstream [release commit `17e17b784ea83e97e84dacb813ccec25072dbd1d`](https://github.com/kamalmostafa/minimodem/tree/17e17b784ea83e97e84dacb813ccec25072dbd1d), 0.24 source archive SHA-256 `f8cca4db8e3f284d67f843054d6bb4d88a3db5e77b26192410e41e9a06f4378e` | External executable/source inspection only. Copyright 2011–2016 Kamal Mostafa; GPL-3.0-or-later. Its encoder/decoder are not copied into Go. |
| Fldigi behavior comparison | Unmodified Debian `4.2.06-1+deb13u1` binary; upstream archive and binary/package hashes in `testdata/manifests/tools.json` | External application comparison, not an extracted modem core. Source archive is upstream 4.2.06; the Debian binary additionally includes its distribution changes. |
| External GRITTY recordings | [Afreet GRITTY demo](https://dxatlas.com/Gritty/Demo/); five original-file SHA-256 values and URLs in `testdata/manifests/gritty.json` | Originals, derived PCM, screenshots used for source review, and decoder captures stay external. The demo includes beta-tester recordings; exact recorder attribution and redistribution permission remain unresolved. No ownership or GPL licensing of these recordings is claimed. No transcript is admitted as ground truth. |
| External W6IWI recordings | [W6IWI RTTY audio](https://w6iwi.org/rtty/audio/), retrieved 4 November 2026; three original-file hashes, six derivative hashes, direct URLs, and source-page snapshot hash in `testdata/manifests/w6iwi.json` | W6IWI describes transmissions from Tucson received at KFS SDR. Originals, converted PCM and decoder captures stay external; no redistribution grant or GPL ownership is claimed. The originals' RIFF size overstatement is documented, not repaired in place. Source-described patterns and decoder output are not admitted as aligned ground truth. |
| External corpus resampling | libsamplerate 0.2.2, Debian `0.2.2-4+b2`; source archive SHA-256 `16e881487f184250deb4fcb60432d7556ab12cb58caea71ef23960aec6c0405a`; library/package pins in `tools.json` | Unmodified external BSD-2-Clause library, copyright Erik de Castro Lopo and upstream contributors. `scripts/corpus.py` declares the public `SRC_DATA`/function ABI from `include/samplerate.h` using ctypes; no resampler implementation or filter coefficients copied. The upstream license is retained in [scripts/NOTICE.libsamplerate](../scripts/NOTICE.libsamplerate), with original notices also in the external package/source archive. The Go module has no native dependency. |
| Task/version conventions | [go-ft8 `cb9e03f171508232d7b1d25cf7323f58a5748233`](https://github.com/ColonelBlimp/go-ft8/tree/cb9e03f171508232d7b1d25cf7323f58a5748233), `Taskfile.yml` and `Taskfile.version.yml` | Read for command naming and separate version workflow. New minimal task definitions; no FT8 backend, dotenv loading, authentication helper, or release implementation copied. GPL-3.0-only. |

Archive download URLs and checksums are machine-readable in
[`testdata/manifests/tools.json`](../testdata/manifests/tools.json). The download
recipe verifies every archive before extraction. No moving branch or package
alias is used to generate fixtures. The selected versions are fixed reference
versions, not claims about the newest upstream release.

The alphabet notice preserves fldigi's credit to its gmfsk origins. Future
source-derived DSP must receive its own provenance entry, affected-file list,
modification description, and retained upstream notices. A translation into Go
would remain source-derived work.
