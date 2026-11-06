<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: controlled impairments

Date: 6 November 2026. This extends the [clean baseline](RECEIVER_BASELINE.md)
without changing the receiver. It characterizes development failures; it does
not complete stage 3 or qualify release reception, squelch, or clock tolerance.
This report records the original one-bit detector. The subsequent
[gain-step timing update](RECEIVER_GAIN_STEPS.md) preserves this evidence and
compares the current detector against it; its common-step losses are now zero.

The [condition manifest](../testdata/manifests/receiver-channel-v1.json) fixes
24 conditions and seeds 7, 23, and 101 before measurement. All nine independent
minimodem recordings remain development sources. The same sources under more
conditions/seeds do not count as additional independent or held-out recordings.
The [retained report](../testdata/reference/receiver-channel-v1.json) contains
324 signal runs and six short noise-only runs, manifest/generator-source hashes,
Go version, little-endian PCM16 hashes, clipped sample counts, actual code arrays,
exact text under both USOS policies, rejected candidates, and separate
substitution/deletion/insertion counts. The source WAV hashes are checked by
the shared fixture loader. No original fixture or expected text was changed.

## Channel definition

Generator `receiver-channel-v1` is implemented in
[`channel_test.go`](../internal/receiver/channel_test.go); the complete source
hash is recorded in the report. The original report's harness hash is
historical; the expanded comparison harness and current receiver sources are
pinned in the newer report. Noise uses Go 1.26.2's `math/rand/v2` PCG,
initialized with `(seed, 0x72747479)`, and `NormFloat64`. Byte identity is
recorded on the clean-baseline Linux amd64 host. A changed runtime or platform
that changes hashes must be investigated before replacing evidence.

The source is `s[n] = PCM[n]/32768`. Every signal case applies fixed gain
`g = 1/16`, giving headroom for noise, with no normalization after impairments.
Output conversion is round-to-even of `32768*x`, followed by saturation to
`[-32768,32767]`; every saturated sample is counted. No trimming, padding,
whitespace cleanup, or text correction is applied. All available output over
the complete derivative is scored against the original complete code/text
expectation. Missing final characters and extra observations therefore count.

- **Tuning:** change the receiver center by the specified Hz, leaving PCM
  unchanged. These are receive tuning errors, not newly synthesized FSK or AFC.
- **Capture clock:** let `r = 1 + ppm/1e6`. Derivative length is
  `floor((N-1)*r)+1`, with output sample `i` linearly interpolated at source
  position `i/r`. Positive ppm stretches the recording and lowers apparent
  tone frequencies and baud together by `1/r`. Linear interpolation is a
  deliberately simple development perturbation, not the qualified corpus
  resampler or an isolated baud-rate stress. Its interpolation error is part
  of these results. Tests separately verify identity and a known ramp.
- **AWGN:** measure full-file signal power `P = mean((g*s[n])^2)`. For target
  SNR `D` dB in reference bandwidth `B=2500 Hz`, use independent zero-mean
  Gaussian samples with variance
  `sigma^2 = P*Fs/(2*B*10^(D/10))`. Full-band white noise variance corresponds
  to power `sigma^2*2*B/Fs` in that bandwidth. Record realized SNR from
  quantized derivative minus its noiseless counterpart, including clipping.
  Calibration tests check a 5 dB case within 0.25 dB at both sample rates.
  Test-channel SNR is unrelated to runtime detector confidence or squelch.
- **Common fade/gain step:** multiply by 0.1 between integer sample indices
  `floor(0.35*N)` and `floor(0.65*N)`; endpoints are abrupt. This is a 20 dB
  common amplitude step, not selective branch fading or a realistic HF channel.
- **Dropout:** replace two nominal bit periods with zeros starting at
  `floor(0.4*N)`. Gap length is rounded to the nearest sample. No discontinuity
  reset is signalled, so this specifically characterizes undeclared loss.
- **Impulses:** add amplitude 0.5 for two samples every 100 ms, alternating
  polarity between impulses. Width is two samples at either rate, so physical
  impulse durations differ. Saturation uses the same counted conversion.

Noise-only cases use five seconds of Gaussian PCM with sigma 0.03, each rate
and each of the three seeds, default baud/mark-high/1.5 stops, and no calibrated
squelch. There is no signal, so no SNR is assigned. Printable output includes
ASCII SPACE through `~`; CR/LF, NUL/BEL and shifts count as control observations.
Raw noise-only codes and rendered text are retained separately from counts.

## Results

Non-noise signal conditions compare 142 reference codes and 100 text bytes per
USOS policy. Each AWGN level compares 426 codes and 300 text bytes across three
seeds. Edit distance has deterministic tie order: substitution, deletion,
insertion. Counts describe the minimum alignment, not an inferred physical
cause. CER is the text edit count divided by reference text length; it can
exceed one when insertions dominate. NUL/BEL/shift information remains in raw
code scoring. No scoring interval is selected after observing failures.

| Condition | Code edits / reference | Text edits USOS off / reference | Text edits USOS on / reference | Clipped samples |
| --- | ---: | ---: | ---: | ---: |
| Reference at fixed gain | 0 / 142 | 0 / 100 | 0 / 100 | 0 |
| Tuning ±5, ±10, ±20 Hz, each | 0 / 142 | 0 / 100 | 0 / 100 | 0 |
| Capture clock ±100, ±1000 ppm, each | 0 / 142 | 0 / 100 | 0 / 100 | 0 |
| All four ±10 Hz / ±1000 ppm combinations, each | 0 / 142 | 0 / 100 | 0 / 100 | 0 |
| AWGN 20 dB | 0 / 426 | 0 / 300 | 0 / 300 | 0 |
| AWGN 10 dB | 0 / 426 | 0 / 300 | 0 / 300 | 0 |
| AWGN 5 dB | 0 / 426 | 0 / 300 | 0 / 300 | 0 |
| AWGN 0 dB | 31 / 426 | 28 / 300 (9.33%) | 28 / 300 (9.33%) | 0 |
| AWGN −5 dB | 154 / 426 | 132 / 300 (44.00%) | 134 / 300 (44.67%) | 0 |
| AWGN −10 dB | 351 / 426 | 277 / 300 (92.33%) | 277 / 300 (92.33%) | 10 |
| Common 20 dB step | 3 / 142 | 3 / 100 | 2 / 100 | 0 |
| Two-bit undeclared dropout | 33 / 142 | 26 / 100 | 25 / 100 | 0 |
| Specified impulses | 0 / 142 | 0 / 100 | 0 / 100 | 0 |

The −10 dB row includes ten clipped samples and is therefore a noisy/clipped
ADC case, not ideal unsaturated Gaussian noise. Realized SNR ranges across
files/seeds are target minus 0.116 dB to target plus 0.073 dB. A single short
three-seed sweep does not locate a general SNR operating threshold.

At 0 dB, code alignment records 31 deletions and no code substitutions or
insertions; rendered text has two substitutions and 26 deletions. This shows
why raw-code and rendered-text evidence are both needed: missing shift/control
observations can change later interpretation. It does not prove the exact
physical cause of every deletion. The common step loses three raw codes;
the dropout has eight code substitutions and 25 deletions. Both deserve
detector/framer recovery work before a production receiver contract is shipped.

The six five-second noise-only runs produce zero printable characters and
one CR observation (12 kHz, seed 7, raw code 8). Thirty seconds across both
rates cannot qualify the required multi-hour per-condition false-output gate.
This control output also demonstrates why a numerical energy floor must not
be advertised as carrier squelch.

Whole, 20 ms, and 127-sample partitions preserve all observations, sample
positions, counters, and final receiver state for every signal run, including
failed reception. Additional one-sample and seeded random partitions check
AWGN 0 dB, the dropout, and one combined tuning/clock case on all nine files
with USOS on. Empty calls are interleaved in the full study. These checks
establish streaming determinism; they do not turn the reception failures into
passing quality results.

## Reproduction and review

```sh
GOCACHE=/tmp/go-build task receiver:impairments:legacy # original outcomes below
GOCACHE=/tmp/go-build task receiver:impairments        # current comparison
GOCACHE=/tmp/go-build task check
# Export a new report to a path that does not exist:
GOCACHE=/tmp/go-build go test ./internal/receiver -run '^TestChannelStudy$' \
  -v -count=1 -args -receiver-study-legacy \
  -receiver-study-output=/tmp/receiver-channel-new.json
```

Ordinary tests reproduce the current report and compare every run against
this baseline. The explicit legacy command checks the original report's
input/output fields while preserving its historical provenance. Go version
is retained without requiring an exact runtime version. The explicit export
flag creates a new file and refuses overwrites.
Review report differences before replacing retained evidence. A passing study
test means reproducibility and invariants held, including the recorded errors.
It is not a reception acceptance result. No decoder coefficients, timing
tolerances, or expected input text were changed to obtain these measurements.
`task check` passed ordinary and CGO-disabled suites, vet, race, version, and
the ten-test offline Python suite, with the optional resampler qualification
skipped because its library was not configured. External reference decoders
were not replayed for this study; existing independently generated WAVs supply
the known inputs.

## HF labels and next decision

The [W6IWI source page](https://w6iwi.org/rtty/audio/) was checked again on
6 November 2026. It describes repeated test text and a prefixed RY pattern,
without the exact control stream or sample-aligned transcript. This confirms
the existing intake limitation. Decoder agreement and recognizable RY output
do not establish the missing alignment or control expectations. The originals
and derivatives remain external, unlabelled development recordings with empty
scoring ranges; no transcript or redistribution permission is inferred.

Admission still needs source-confirmed transmitted content or a separately
reviewed transcript, explicit uncertain spans, independently checked sample
boundaries, measured signal/framing settings, and fixed scoring views. Preserve
decoder captures separately; split all variants of one recording together.
New independent recording groups are needed for held-out evaluation. No
messages were sent to recording authors and no recording was relabelled.

Continue with stronger detection/threshold handling and recovery experiments
using this fixed failure baseline, then run scored HF comparisons against pinned
fldigi once labels are admitted. The common amplitude step is a useful first
regression target. Selective fading, adjacent RTTY/interference, isolated baud
stress with long independent traffic, long-session numerics/memory, calibrated
squelch, full noise-only hours, and the held-out comparator gate remain open.
The experiment provides no evidence requiring a native backend yet.
