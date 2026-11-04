<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Fixture corpus

This is a small **development fixture set**, not the release reception corpus.
There is no go-rtty receiver or transmitter yet. Passing the ordinary tests
establishes artifact integrity and consistency, not modem interoperability.

## Checked-in evidence

| Group | Contents | Evidence |
| --- | --- | --- |
| Protocol | All 32 US alphabet codes in letters/figures, explicit LSB-first bits; eight literal half-bit frame examples; eight text and four unsupported-rune examples | Independently specified before implementation and checked against pinned source tables. Future codec tests must consume these fixed expectations. |
| Clean audio | Eight minimodem transmissions: 12/48 kHz × mark-high/mark-low × 1.5/2 stop bits | WAV checksums, complete raw-code and exact text captures; 17 frames each, including shifts and CR/LF. |
| USOS probe | `A1 A`, six frames, 12 kHz, mark-high, 1.5 stops | The transmitted codes are `[31,3,27,23,4,3]`. Expected text is `A1 A` with receive USOS on and `A1 -` with it off. |
| Second receiver | All nine WAVs replayed in unmodified fldigi, with both USOS policies | 18 exact widget-text comparisons; full captured output and configuration hashes in `testdata/reference/fldigi-results.json`. CRLF is displayed as LF. |
| External corpus manifest | Five GRITTY originals and ten 12/48 kHz derivatives; audio and captures stored externally | Source/derived hashes, provisional tuning, conversion recipe, and development-only assignments in `testdata/manifests/gritty.json`. No trusted labels or scored reception result. |
| W6IWI external manifest | Three HF originals and six 12/48 kHz derivatives; audio and captures stored externally | Source-described test patterns, hashes, provisional tuning, and an explicit WebSDR container exception in `testdata/manifests/w6iwi.json`. No trusted aligned transcripts or scored ranges. |

Minimodem's TX USOS behavior can omit an explicit LTRS after a space. Its
output must not be used as the expected code stream of the planned go-rtty
encoder, which has its own specified shift policy. Minimodem also prints NUL
as a debug character and permits some input substitutions. The clean audio
fixtures avoid those behaviors; the protocol vectors preserve the library's
strict intended contract.

## Manifests and scoring

`testdata/manifests/tools.json` pins reference source archives, binary packages,
executables, and the isolated Linux runtime packages used for reproduction.
`artifacts.json` records SHA-256 checksums for the literal vectors and source
text. `independent.json` records each WAV, input, executable identity, settings,
complete expected code stream, expected text for both USOS policies, and
captured minimodem output.

WAV files are mono signed PCM16 in RIFF containers. Sample counts exclude the
WAV header. All audio is scored from sample zero to the end; there is no
preamble trimming or uncertain transcript span. Lead and tail are real mark
audio from minimodem, two bits each. No synthetic flush or post-generation
padding was added.

The requested baud is `909/20` (45.45). Minimodem 0.24 rounds each bit to the
nearest sample: 264 samples at 12 kHz, 1056 at 48 kHz. In this subset the
effective bit time is therefore exactly 22 ms (`500/11` baud). Stop lengths
are 396/528 samples at 12 kHz and 1584/2112 at 48 kHz. These files do **not**
test the planned rational accumulator's fractional-sample timing.

Command arguments in each manifest use a single `{wav}` path placeholder.
The configuration SHA-256 hashes compact UTF-8 JSON of `[tx_args, rx_args]`,
in that order, with no trailing newline. Capture files retain original text,
five-bit words in wire order, and diagnostic output. Confidence diagnostics
are tool metrics, not probabilities of correctness.

Ordinary Go tests validate file hashes, WAV structure, sample counts, argument
consistency, capture contents, and vector structure. They run offline and do
not launch external tools. `task reference:minimodem` regenerates into a
temporary directory, requires identical WAV bytes, and compares live text and
raw-code output to the fixed expectations. It never rewrites the corpus.

## Candidate recording sources

Source review: 4 November 2026. These are development sources and acquisition
leads, not labelled reception fixtures or acceptance results. No off-air audio or third-party transcript has
been added to the repository. Page descriptions and WAV headers do not establish
transmitted text, measured signal parameters, or full-file integrity.

| Source | Evidence and intended use | Outstanding work |
| --- | --- | --- |
| [GRITTY demo](https://dxatlas.com/Gritty/Demo/) | Five complete off-air WAVs acquired into an external development cache, with original and 12/48 kHz derivative hashes in `testdata/manifests/gritty.json`. | Establish reviewed transcripts, exact signal settings, and redistribution terms. The page invites download and testing but supplies no explicit redistribution grant. |
| [W6IWI RTTY audio](https://w6iwi.org/rtty/audio/) | Three complete HF test recordings and six converted WAVs acquired externally; hashes and source descriptions in `testdata/manifests/w6iwi.json`. | Align and independently review the source-described patterns, establish controls and timing, and resolve redistribution terms. Inspected recordings are development material. |
| [LU3FZR TNC test set lead](https://www.qsl.net/l/lu3fzr/) | Operator reports 1993 clean 170 Hz recordings at 45, 50, 57, 75, 100, 110, 150, 200, and 300 baud, with companion text. Potential independent rate/framing fixtures. | Page retrieval failed, including HTTP 403; filenames, contents, framing, exact rates, and terms remain unverified. Check whether each text file is transmitted input or decoder output. Most listed rates are outside v0.1. |
| [brainwagon: The sights and sounds of RTTY](https://brainwagon.org/2011/07/02/the-sights-and-sounds-of-rtty/) | Audio-Digital's directory supplied this specific article URL on 4 November. Earlier lead described roughly one minute of SDR-IQ contest audio near 14.090 MHz. | Direct article retrieval still failed; audio URL, transcript, format, and terms remain unverified. The directory excerpt is not primary recording evidence. |
| [Signal Identification Wiki RTTY samples](https://www.sigidwiki.com/wiki/RTTY) | Redirects to Radio Teletype (RTTY); 16 baud/shift examples plus overlapping-signal and contest MP3s. Three downloads and media formats verified below. | Transcripts, recording origin, measured signal parameters, and redistribution terms remain unverified. These are candidate fixtures, not labelled reception coverage. |
| [F1ULT / MultiPSK RTTY](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_en.htm) | Two MP3 examples labelled RTTY 45 and RTTY 50, with waterfall images and MultiPSK mode documentation. Both audio downloads verified below. | No transcript, recording provenance, or redistribution grant identified on the page. Its documented text mapping differs from the planned go-rtty profile. |
| [benj500, Freesound RTTY.wav](https://freesound.org/people/benj500/sounds/369476/) | Primary page describes a DX394 HF recording and lists CC0, 15.007 s, stereo PCM16, 48 kHz. Found through [Audio-Digital](https://www.audio-digital.net/r-pages/rtty-audio-samples.html). | Original download requires login; not downloaded. No transcript or measured modem parameters. Page metadata is not file verification; any future stereo conversion needs an explicit channel policy. |
| [RTTYOnline](https://rttyonline.com/) / [RTTY.COM](https://www.rtty.com/itty/index.htm) | Feed directories and a [KD2PEP custom-text WAV generator](https://unluckyfett.github.io/TTY-ART-GENERATOR/) provide sustained-stream and independent-transmitter leads. | No live stream capture or generator validation performed. Pin generator revision, text mapping, timing, and runtime before admitting generated fixtures. |
| Operator captures via KiwiSDR/WebSDR or a local receiver | Can supply longer sessions, different transmitters, and selected channel conditions. | Preserve capture provenance, receiver settings, permission terms, and separately reviewed text. A receiver recording alone does not provide ground truth. |

The GRITTY links and advertised conditions are:

| WAV | Page description |
| --- | --- |
| [SX150ITU_2015-04-13 135142.wav](https://dxatlas.com/Gritty/Demo/SX150ITU_2015-04-13%20135142.wav) | Strong CQ with quiet intervals. |
| [II1SAP_2015-04-13 131007.wav](https://dxatlas.com/Gritty/Demo/II1SAP_2015-04-13%20131007.wav) | CQ affected by fading. |
| [R120RB_2015-04-13 125938.wav](https://dxatlas.com/Gritty/Demo/R120RB_2015-04-13%20125938.wav) | Weak signals with deep fading. |
| [PA4PS_2015-03-22 164743.wav](https://dxatlas.com/Gritty/Demo/PA4PS_2015-03-22%20164743.wav) | BARTG contest; multiple signals in the passband. |
| [DL2AL_2015-03-22 171224.wav](https://dxatlas.com/Gritty/Demo/DL2AL_2015-03-22%20171224.wav) | BARTG contest; heavy receiver clipping. |

Complete downloads verified mono uncompressed PCM16 at **6,000 Hz**, with
359,936 frames (about 59.989 seconds) in each original. Each original is
720,006 bytes including RIFF metadata. The
[external manifest](../testdata/manifests/gritty.json) now pins full-file
checksums and deterministic 12/48 kHz derivatives. The
[fetch, conversion, and replay recipe](REFERENCE.md#external-gritty-development-workflow)
records the fixed 0.5 conversion gain, filter/timing policy, and reference
settings. This is development evidence only; scoring ranges remain empty.

GRITTY's page describes callsign/exchange post-processing and shift correction.
Its screenshots and 2Tone's output are comparison evidence, not trusted
transmitted text. Crowded-passband and severe-fading cases can inform the
separately reported stretch evaluation without changing the required v0.1 gates.

For a later profile, DWD's published
[Program 1 schedule](https://www.dwd.de/EN/specialusers/shipping/broadcast_en/broadcast_rtty_1_092023.pdf?__blob=publicationFile&v=2)
lists 50 baud, deviation of plus/minus 225 Hz (450 Hz tone separation), and
00:00–24:00 UTC operation. This is a capture lead, not a verified live recording.
It is outside v0.1; 450 Hz is also distinct from the design's proposed v0.2
425/850 Hz shifts. Using it as a supported profile would require a separate
scope decision and fixtures.

### Signal Identification Wiki downloads

The supplied [RTTY URL](https://www.sigidwiki.com/wiki/RTTY) exposes MP3 links
in its HTML audio elements. Three files were downloaded into a temporary
directory on 3 November 2026 and inspected with `ffprobe`; their SHA-1 values
matched the wiki's upload metadata. No audio was added to the repository.

| Direct sample | Approximate duration | Encoded audio format |
| --- | ---: | --- |
| [170 Hz / 45.45 baud example](https://www.sigidwiki.com/images/3/3a/RTTY_170Hz_45.45Bd.mp3) | 11.55 s | MP3, mono, 44,100 Hz |
| [Overlapping RTTY signals](https://www.sigidwiki.com/images/7/7e/OverlappingRTTY_Sound.mp3) | 16.38 s | MP3, mono, 32,000 Hz |
| [Contest, 15 August 2015, 14.0874 MHz](https://www.sigidwiki.com/images/8/89/RTTY_contest_2015-08-15T18-48-13Z_14087.4kHz.mp3) | 36.50 s | MP3, mono, 8,000 Hz |

Durations are `ffprobe` reports, not scored spans. Baud/shift and recording
descriptions above come from the page and filenames, not signal measurements.
The page's main audio player links a separate lowercase-`bd` filename; the
verified 45.45-baud sample is the uppercase-`Bd` file in its sample matrix.

The three file-description pages were empty, and the wiki API returned empty
site-license fields. Neither provides a transcript or establishes permission
to redistribute these files. Upload identities alone do not identify recording
authors. The overlapping and contest clips merit further investigation beyond
the short format examples, but none currently contributes scored characters.
Admission needs a pinned MP3-to-PCM decode and resampling recipe for 12/48 kHz.
Lossy compression is part of this source path; seek original lossless audio
where available and retain its identity separately.

### F1ULT / MultiPSK downloads

The supplied [RTTY page](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_en.htm)
was retrieved over HTTP on 3 November 2026; the browser's HTTPS attempt failed.
Both MP3s were downloaded into temporary storage and inspected with `ffprobe`.
Each is mono at 11,025 Hz, with a reported duration of approximately 10.30 s.

| Page label | Direct audio | Accompanying image |
| --- | --- | --- |
| RTTY 45 | [RTTY_45.mp3](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_fichiers/RTTY_45.mp3) | [Waterfall](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_fichiers/RTTY_45.gif) |
| RTTY 50 | [RTTY_50.mp3](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_fichiers/RTTY_50.mp3) | [Waterfall](http://f1ult.free.fr/DIGIMODES/MULTIPSK/RTTY_fichiers/RTTY_50.gif) |

Visual inspection confirmed that the images contain waterfalls; neither supplies
a decoded transcript. Exact baud, shift, polarity, and framing have not been
measured from the audio. In particular, the label "45" does not resolve exact
`500/11` versus `909/20` timing. The 50-baud example is outside v0.1. Both files
need pinned MP3 decoding and resampling before use with the planned PCM input.

The page describes MultiPSK-specific text behavior: for example, figures code
26 maps to `@`, code 30 to `=`, and LF is ignored. The planned `AlphabetUS`
uses `&` and `;` respectively and preserves LF. Consequently, adopting MultiPSK
output as a comparator would require explicit alphabet/control settings and a
declared scoring view. These examples do not change the go-rtty codec contract
or the existing fldigi/minimodem reference configuration.

### W6IWI intake

Complete downloads on 4 November 2026 verified mono PCM16 at **8,000 Hz**.
The [manifest](../testdata/manifests/w6iwi.json) pins the original bytes, all six
12/48 kHz derivatives, and the retrieved source-page hash. Audio remains in
the external cache; it has not been added to the repository.

| Recording | Original frames | Duration (s) | Provisional mark / space (Hz) |
| --- | ---: | ---: | ---: |
| `w6iwi-qbf-40m-20220315` | 12,545,280 | 1,568.160 | 2050 / 2220 |
| `w6iwi-qbf-20m-20220614` | 18,604,800 | 2,325.600 | 2125 / 2295 |
| `w6iwi-ry-170-20240415` | 4,525,056 | 565.632 | 2125 / 2295 |

The source describes repeated Quick Brown Fox text for the first two files,
and an RY pattern with a station prefix for the third. These descriptions do
not specify exact control codes or sample-aligned expected text. Exploratory
spectra and minimodem captures support the provisional tuning above. In
particular, the 40 m signal is below the nominal 2125/2295 Hz pair. The fixed
45.45-baud and 1.5-stop-bit comparator settings are not measurements of the
mechanical transmitter's exact rate or framing.

All three originals have an eight-byte RIFF length overstatement. Their
`fmt ` and `data` chunks, declared sample counts, and complete PCM payloads
were inspected. The manifest explicitly annotates this exception; the reader
still rejects truncated PCM. Originals remain unchanged. Derived WAVs have
standard headers and use the pinned resampler with fixed 0.5 gain, with no
normalization, trimming, padding, or frequency correction.

Full minimodem captures preserve text and raw codes separately from any future
labels. NUL debug output, dropped characters, and shift errors in those captures
must not become expected transmitted text. All three recording groups remain
development-only with empty scoring ranges. The
[workflow and validation record](REFERENCE.md#external-w6iwi-development-workflow)
distinguish completed captures from unperformed checks.

## Admission and acquisition order

The five GRITTY and three W6IWI originals are acquired and assigned to development
because they were inspected during harness bring-up and intake. Next review and
align W6IWI's test patterns, obtain GRITTY labels, and pursue independent amateur
captures; reserve new recording groups for held-out evaluation before tuning.
Preserve an entire original recording before
making derived audio or excerpts. For each candidate:

1. Record the source URL, retrieval date, recorder/transmitter provenance,
   applicable terms, and original-file SHA-256. Keep recordings external when
   redistribution is unsuitable, as allowed by the design; repository inclusion
   requires recorded permission and a [provenance entry](PROVENANCE.md).
2. Verify PCM format, duration, baud, shift, audio polarity, framing, alphabet,
   and relevant receiver settings. Select a target signal for crowded audio.
   Record unknowns explicitly instead of filling them from modem defaults.
3. Establish known transmitted text or a separately reviewed transcript, with
   uncertain spans excluded using fixed scoring ranges. Preserve raw reference
   output separately. Decoder agreement alone does not establish ground truth.
4. Pin conversions and comparator settings, including USOS, AFC, and squelch.
   The existing fldigi adapter covers the current clean fixtures; arbitrary
   recordings need explicit configuration and validation before comparison.
5. Freeze development/held-out assignments and condition groups before receiver
   tuning. Keep excerpts and converted/impaired variants of one recording in
   the same partition; they do not count as distinct recordings.

For controlled impairments, extend the existing pinned minimodem workflow and
add independently generated fldigi audio. A quick `minimodem --tx rtty` example
is useful for exploration; regression fixtures need explicit settings, source
text, tool hashes, and captures as in [REFERENCE.md](REFERENCE.md). Noise mixing
also needs a pinned generator, fixed seed, power/scaling equations, clipping
policy, and the design's 2500 Hz reference bandwidth for SNR. Generated audio
does not replace the labelled off-air corpus.

## Updating and extending

Treat changes to expected values, source versions, or settings as reviewed
fixture changes. Preserve the source text, full commands, archive/executable
hashes, runtime versions, and actual output. Investigate waveform differences
before updating checksums; a pinned binary still uses host math/audio libraries.

Before receiver tuning, obtain trusted labelled HF recordings with explicit
redistribution terms and freeze development/held-out assignments, required
conditions, and scoring ranges. No such recordings have been acquired in this
checkout. The design's 20-recording/30,000-character requirement, noise-only
hours, timing stress, impaired channels, and performance gates remain unrun.

Reference reproduction and current limitations are in [REFERENCE.md](REFERENCE.md).
