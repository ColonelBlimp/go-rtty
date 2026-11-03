<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Reference workflow

The foundation uses minimodem 0.24 for independent audio and raw-code capture,
and the **unmodified fldigi 4.2.06 application** for a second receiver check.
There is no go-rtty modem yet. These checks establish a reproducible reference
workflow, not bidirectional interoperability with this library.

## Pinned tools

The tested Linux amd64 packages are minimodem `0.24-1+b2` and fldigi
`4.2.06-1+deb13u1` from the Debian archive. External corpus conversion also uses
libsamplerate `0.2.2-4+b2`, loaded by the Python development tool only; it adds
no Go or CGO dependency. Source archive, package, and
executable SHA-256 values are in
[`tools.json`](../testdata/manifests/tools.json). A version string alone is
insufficient: verification rejects an executable with a different hash.

The recipe extracts packages into a caller-selected directory without a system
installation. It requires Python 3.12 or newer, `ar`, compatible host libraries,
and, for the fldigi adapter, X11/XTest and `xkbcomp`. The lock includes the
FLTK, Hamlib, mbedTLS, and Xvfb packages needed in the tested environment.
It is not a complete container image or an all-platform dependency installer.

```sh
python3 scripts/reference.py fetch /tmp/go-rtty-tools

export RTTY_MINIMODEM=/tmp/go-rtty-tools/root/usr/bin/minimodem
export RTTY_FLDIGI=/tmp/go-rtty-tools/root/usr/bin/fldigi
export RTTY_XVFB=/tmp/go-rtty-tools/root/usr/bin/Xvfb
export RTTY_SAMPLERATE=/tmp/go-rtty-tools/root/usr/lib/x86_64-linux-gnu/libsamplerate.so.0.2.2
export LD_LIBRARY_PATH=/tmp/go-rtty-tools/root/usr/lib/x86_64-linux-gnu${LD_LIBRARY_PATH:+:$LD_LIBRARY_PATH}

task reference:minimodem
task reference:fldigi
# Or run both:
task test:oracle
```

Downloading happens only in the explicit `fetch` command. No Go test, Task
test, or ordinary CI job fetches these tools. Source archives are retained for
inspection; this recipe runs the fixed distribution binaries, not a local
rebuild. The fldigi distribution build flags are retained with the validation
evidence. The underlying upstream source archive does not include Debian's
distribution patches; the binary package identity is the execution pin.

## Minimodem configuration and reproduction

Every WAV's exact TX and RX argument arrays appear in
[`independent.json`](../testdata/manifests/independent.json). They explicitly
select five-bit LSB-first encoding, one start bit, stop length, sample rate,
mark/space frequencies, and 45.45 baud. TX amplitude is 0.5 and the sine lookup
table has 4096 entries. RX bandwidth is 10 Hz, minimum confidence 1.5, and
search limit 2.3. Automatic carrier detection and inverted start/stop framing
are not enabled.

The input is an ordinary file with known bytes. A delayed stdin pipe can cause
minimodem to insert idle tones; the adapter deliberately uses a regular file.
Regeneration happens in a temporary directory and must reproduce the WAV
checksum before the live text and raw-code comparisons run.

In this pinned source, both receive and transmit unshift-on-space behavior are
always enabled and have no command-line switch. The source's original RTTY
defaults are 45.45 baud, 1.5 stops, 48 kHz, mark 1585 Hz, space 1415 Hz,
amplitude 1, and 4096 sine entries. The manifest overrides the applicable
settings; it records the per-bit timing rounding discussed in
[CORPUS.md](CORPUS.md).

## Fldigi offline playback and complete capture

`scripts/fldigi-replay.py` starts its own Xvfb display and a fresh application
instance for **each fixture and USOS policy**. It supplies separate temporary
home/configuration directories, file-only audio (`AUDIOIO=3`), and localhost
control ports. It verifies the instance's configuration directory over XML-RPC
before changing settings, selects RTTY and receive-only operation, disables
AFC and squelch for these clean cases, and sets center frequency and polarity.

The adapter uses File → Audio → Playback in the actual application, selects
the unmodified WAV, and disables looping. It then waits for the complete WAV
plus one second of application processing before capturing the entire receive
widget. It does not extract the modem core, modify fldigi, or assume that the
standard binary has a headless WAV command. The source has an optional
benchmark-only build mode, which is not used here.

The pointer coordinates are specific to the pinned FLTK build, its default
font, an 800×460 window at +50+50, and the private 1100×850 X display. This is
a Linux reference adapter, not a portable UI API. A window/font/library change
may make the check fail; fix and requalify the adapter rather than weakening
the output comparison. A real display or audio device is not needed.

Explicit XML settings select US figures, baud index 1 (45.45), shift index 3
(170 Hz), five data bits, no parity, the fixture's stop length, uppercase
output, Kahn demodulation, 68 Hz receive bandwidth, and filter factor 1.25.
RX USOS is tested off and on; TX USOS is explicitly off because this is a
receive check. The original source defaults are RX/TX USOS on, baud index 0
(45 baud), and automatic CRLF on; the test overrides them.

The GUI's receive widget collapses CRLF to LF. Its scoring view is declared
before comparison: complete text with each expected CRLF represented by LF,
with no other trimming, case folding, whitespace removal, or error correction.
Minimodem's separate captures preserve the exact protocol CR/LF bytes and raw
shift codes. Fldigi widget captures do not establish raw-code equivalence.

To retain a new run without replacing the checked-in evidence:

```sh
python3 scripts/fldigi-replay.py --output /tmp/fldigi-new-results.json
# Narrow diagnostic run, still testing both USOS policies:
python3 scripts/fldigi-replay.py --fixture usos-12000-mark-high-stop3
```

The output path must not exist. Each successful result records WAV and
executable hashes, the exact initial XML configuration and its hash, checked
receiver state, expected widget text, and actual captured text. A missing tool,
startup problem, unexpected state, or text mismatch fails the explicit oracle
task. Missing reference tools are not reported as passing tests.

## External GRITTY development workflow

[`gritty.json`](../testdata/manifests/gritty.json) pins five original WAVs and
ten derivatives, together with provisional receive settings. These recordings
have no trusted transcripts yet. Every recording and derivative is assigned to
development; all scoring ranges are empty. Captures are unscored observations,
not expected text, passing reception tests, or held-out evidence.

After fetching the pinned tools and setting the variables above:

```sh
export RTTY_CORPUS=/tmp/go-rtty-corpus
task corpus:fetch       # the only corpus command that accesses the network
task corpus:prepare     # offline; produces and checks both 12 and 48 kHz WAVs
task corpus:verify      # offline; checks all originals and derivatives
task corpus:replay      # full 12 kHz playback, both receive USOS policies
```

Run these sequentially. The cache must be outside the repository, including
after resolving symlinks. Originals live under `original/`, derivatives under
`prepared/`, and captures/logs under `reports/`. Existing audio is verified,
never silently repaired or replaced. A missing file or hash/format mismatch
fails preparation, verification, or replay. The fetch task accepts only the
manifest's complete-file hashes and sizes; ordinary tests never download audio.

Conversion uses the pinned libsamplerate shared library through its public
`src_simple` API, `SRC_SINC_BEST_QUALITY`, mono, and `end_of_input=1`. A fixed
0.5 input gain supplies approximately 6.02 dB of headroom for interpolation;
there is no normalization, frequency shift, noise reduction, or synthetic mark
padding. The converter rejects float output reaching full scale and uses
`src_float_to_short_array` for PCM16 rounding. It requires all input frames to
be consumed and exactly 2 or 8 output frames per original frame. Each generated
WAV must match its fixed hash, including on repeated preparation.

The offline converter handles its filter latency internally. No extra samples
are trimmed; an original boundary at sample `n` maps to `2*n` or `8*n`.
Impulse-position, frame-count, and interior DC-gain checks qualify this mapping
at both rates. Filter edge transients remain in the files; future scoring
ranges must declare any exclusions before reception evaluation. Byte identity
is qualified on Linux amd64 with the pinned library; a different math/runtime
environment that changes output fails the hash check and needs investigation.

The selected mark/space pairs target the station named by each demo file.
Exploratory spectra and pinned minimodem output informed tuning; in the crowded
and clipped files, selecting the strongest pair would select another station.
The manifest records these as provisional settings, not measured baud/framing
or validated transcripts. Fldigi uses the same explicit US alphabet, 45.45
baud, 170 Hz shift, 1.5 stops, filter settings, and disabled AFC/squelch as the
clean workflow, with per-file carrier and mark-low polarity.

`corpus:replay` uses the unmodified application and captures the entire widget,
including noise output. Its JSON records raw widget bytes as base64, text,
configuration and executable hashes, original/derived identities, and checked
receiver state. It emits `captured-unscored`, zero scored characters, and no
reception `pass` field. Each recording is replayed from sample zero with fresh
state for USOS off and on. Logs and temporary application configuration are
retained in a unique external run directory; failures retain completed captures
and an error report. A report path that already exists is refused.

The default report is `$RTTY_CORPUS/reports/gritty-12000.json`. For a new run,
a selected recording, or 48 kHz input, use the script directly:

```sh
python3 scripts/corpus.py replay --output reports/second-run.json
python3 scripts/corpus.py replay --fixture pa4ps_2015-03-22-164743 \
  --sample-rate 48000 --output reports/pa4ps-48000.json
```

`task corpus:check` runs offline acquisition/cache failure checks and runs the
resampler qualification when `RTTY_SAMPLERATE` is set. Otherwise qualification
is explicitly skipped. `task check` and ordinary CI include these Python tests;
neither requires external audio or a GUI. The ordinary Go suites validate the
manifest's pins, format/timing relationships, and unlabelled development status.

## Validation record and remaining gates

The foundation was exercised on 2 November 2026, Linux amd64, Fedora 44,
Go 1.26.2, and Task 3.54.0. Runtime versions were libsndfile 1.2.2,
libsamplerate 0.2.2, FFTW single precision 3.3.10, FLTK 1.3.11, and Hamlib
4.6.2. Binary hashes and captured settings take precedence over version labels.

The retained results cover nine minimodem WAV regenerations with identical
bytes and exact text/raw-code decoding, and 18 fldigi replays (nine fixtures
with RX USOS off and on). See the
[fldigi capture report](../testdata/reference/fldigi-results.json) and
[distribution build information](../testdata/reference/fldigi-build.txt).
The USOS probe produces `A1 -` with USOS off and
`A1 A` with it on. The ordinary Go and CGO-disabled suites, vet, and race
checks pass for the foundation package. The added GitHub Actions workflow has
not been run on GitHub as part of this local work.

On 3 November 2026 the external GRITTY workflow verified all five complete
downloads and all ten derived WAVs. The pinned resampler passed impulse
alignment, duration, and DC-gain qualification at both 12 and 48 kHz. All ten
full 12 kHz fldigi captures completed (five recordings, both USOS policies),
with configuration/state, raw/text capture, and provenance hashes checked.
The report is external at `reports/gritty-12000.json` beneath the corpus cache;
the retained run's SHA-256 is
`af143db1403d6a53d5d333acea0302bb016a017e9542466d0218702659aa1dfd`.
Its outputs remain unscored and are not trusted transcripts. Full off-air
replay of the 48 kHz derivatives has not been run.

The changed adapter also passed all 18 existing clean fldigi comparisons, and
all nine minimodem fixtures reproduced exactly with the pinned library path.
`task check` passed, including ordinary and CGO-disabled Go tests, vet, race,
and all eight Python tests with the resampler configured. Without that library
configured, the offline Python suite passed with its one qualification test
explicitly skipped. Re-extraction from cached packages reproduced all three
pinned tool/library hashes. The updated CI definition remains locally checked
only; it has not been run on GitHub.

Still unperformed: trusted labelled HF recordings and held-out reception
comparisons, go-rtty protocol/DSP/streaming implementation and chunk invariance,
fldigi-generated TX fixtures, go-rtty bidirectional interoperability, transmit
spectral qualification, false-output hours, and performance/resource gates.
No native backend or public package is introduced by this foundation. Five
GRITTY originals have now been acquired in an external development cache;
trusted labels, held-out recordings, and redistribution terms remain unresolved.
Stage 1's labelled-corpus work is incomplete. The [candidate-source review and
admission plan](CORPUS.md#candidate-recording-sources) records the remaining
leads and transcript/access/permission work.
