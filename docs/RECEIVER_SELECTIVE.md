<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: unequal tone levels and threshold correction

Date: 6 November 2026. This is a development comparison of three detectors,
not a production receiver or qualification of HF reception. The current
adaptive-edge receiver remains unchanged. Both threshold-correction candidates
remain test-only because their gains on unequal-tone audio come with
regressions on the existing impairment matrix.
The later [guarded-envelope study](RECEIVER_NOISE_AWARE.md) retains this
evidence and adds noise/evidence gates plus a half-bit qualification variant.
That variant recovers most unequal-tone losses without code/text regression
on the previous matrix; it also remains test-only.

The [manifest](../testdata/manifests/receiver-selective-v1.json) fixes eleven
conditions, seeds 7/23/101, and candidate constants before measurement. The
[retained report](../testdata/reference/receiver-selective-v1.json) includes:

- 405 selective-amplitude results: 135 inputs evaluated by three candidates.
- 972 regression results: the previous 324 impairment inputs per candidate.
- 72 negative-control results across sample rates, polarities, and candidates.
- Manifest, prior report, source, and derivative PCM hashes; actual codes and
  both exact text views; substitutions/deletions/insertions; rejected framing;
  clipping counts and selected valid-frame delay measurements.

All inputs remain development material derived from the same nine independent
minimodem WAVs. Candidate copies, seeds, and derivatives do not add independent
transmitters or held-out recordings. Original audio, code/text expectations,
and all earlier evidence reports are unchanged. No external decoder or HF
recording was replayed for this comparison.

## Controlled amplitude model

The model preserves the independently generated PCM waveform, oscillator
phase, duration, and real lead/tail. It obtains logical mark/space intervals
from the original transmitter's recorded lead/bit/stop lengths and the fixed
CSV wire-bit strings. It does not call the production encoder/framer or use
recovered receiver decisions. The detector receives only the resulting PCM.

This is a **symbol-conditioned amplitude model**, not a frequency-selective
convolution, RF multipath channel, or independent new transmitter. It cannot
establish performance under filter dispersion, simultaneous interference, or
physical selective fading. Its label is explicit so these controlled results
cannot silently become HF acceptance evidence.

For a branch attenuation `D` dB, its target amplitude gain is `10^(-D/20)`;
the other branch targets one. At every source sample, the known emitted
state selects the target. An amplitude envelope follows that target with
`g[n] = g[n-1] + (1-exp(-1/(0.0005*Fs)))*(target-g[n-1])`.
The static model starts at the mark gain, matching the real mark lead-in.
This 0.5 ms time constant avoids instantaneous amplitude jumps at bit edges
but remains a specified development perturbation, not a channel estimate.

The fading variants start at gain one. For normalized time
`u = (n/N - 0.25)/0.5`, depth is `(1-cos(2*pi*u))/2` over `0 <= u <= 1`,
and zero elsewhere. The selected branch's target is
`1-(1-10^(-D/20))*depth`; the other remains one. Thus the 20 dB fade is deepest
at the file midpoint and returns smoothly over the central half of the file.
The same causal amplitude envelope follows this target.

Signal gain before impairments is fixed at 1/16. The two 12 dB imbalance/noise
conditions add Gaussian noise *after* amplitude attenuation. Target SNR is
10 dB relative to the complete **unfaded** scaled source power `P` in 2500 Hz:
`sigma^2 = P*Fs/(2*2500*10^(10/10))`. A faded branch consequently has lower
local SNR; no post-fade normalization restores its level. PCG seeds are
`(seed,0x73656c656374)` and noise is Go 1.26.2 `NormFloat64`. Conversion is
round-to-even of `32768*x`, with counted saturation to PCM16. No generated
sample in this study clipped. Calibration verifies identity and steady-tone
20 dB attenuation within 0.05 dB at both rates and branch assignments.

## Candidate definitions

Every candidate shares the existing one-bit correlators, conditional short
timing path, raw energy gate, and framing tolerances. A test-only replay wrapper
observes their PCM-derived branch energies and feeds a separate framer and
renderer. It neither modifies the current receiver nor receives transmitted
labels. Its `ratio` mode reproduces every prior frozen result, and an additional
test checks identical clean observations, counters, and full framing state.

Let `Em` and `Es` be the long-window energies divided by window length squared.
Independent energy peaks update on each input sample:
`Qm=max(Em,Qm*exp(-1/(16*T_samples)))`, likewise `Qs`. Attack is immediate;
energy release is sixteen bit periods. Reset clears both peaks. Constants
were fixed before the study and were not retuned after observing failures.

- **Ratio:** the unchanged `(Em-Es)/(Em+Es)` discriminator, with the same
  zero handling as the current receiver. Peak estimates have no effect.
- **Bounded normalization:** divide each energy by
  `max(its_peak,max(Qm,Qs)/16,1e-8)`, then take their normalized difference.
  The relative floor limits weak-branch amplification; it is not noise-relative
  squelch or a correctness estimate.
- **Envelope offset:** with `Am=sqrt(Em)` and `As=sqrt(Es)`, use
  `(Am-As-0.5*(sqrt(Qm)-sqrt(Qs))) / max(sqrt(Qm)+sqrt(Qs),1e-8)`.
  This subtracts a midpoint envelope bias instead of normalizing each branch.

The same correction applies to the selected edge metric. The wrapper retains
a matching delay ring of short-window total energy, so the selected metric's
branch energies can be reconstructed without transmitter information. Peak
estimates are causal at the current input sample. Nominal delay compensation
does not turn their evolving bias into calibrated timestamp accuracy.

## Unequal-tone results

Each deterministic condition compares 142 codes and 100 text bytes per policy
across nine files. Each noisy condition uses three seeds, comparing 426 codes
and 300 bytes per policy. All available output is scored against unchanged,
complete expectations, with no trimming or text cleanup. Edit-distance tie
order remains substitution, deletion, insertion.

| Condition | Ratio code edits | Bounded normalization code edits | Envelope offset code edits |
| --- | ---: | ---: | ---: |
| Control | 0 | 0 | 0 |
| Mark −6 dB | 75 | 0 | 0 |
| Space −6 dB | 0 | 0 | 0 |
| Mark −12 dB | 75 | 0 | 0 |
| Space −12 dB | 0 | 0 | 0 |
| Mark −20 dB | 142 | 0 | 0 |
| Space −20 dB | 134 | 0 | 0 |
| Mark fade, 20 dB depth | 25 | 0 | 0 |
| Space fade, 20 dB depth | 19 | 18 | 0 |
| Mark −12 dB with 10 dB unfaded-reference SNR | 421 | 29 | 0 |
| Space −12 dB with 10 dB unfaded-reference SNR | 313 | 98 | 0 |

Across these conditions, the ratio has 1204 code edits; bounded normalization
has 145, and envelope offset has zero. Envelope offset recovers all 2130
expected codes and 1500 text bytes per USOS policy. These are comparisons on
one controlled development model, not a general SNR or fading operating limit.

Delay is reported only when raw codes match completely and start estimates
are within half a nominal bit of their indexed known frame. Incorrect or
unaligned runs have no delay value; they are not assigned nearby frames to
produce a plausible latency. Max observed availability after known frame end
is about 25.3 ms for eligible ratio runs, 23.5 ms for bounded normalization,
and 23.6 ms for envelope offset. These short conditional measurements exclude
device/callback delay and do not qualify latency percentiles.

## Existing regression matrix and controls

The previous 324 inputs compare 5112 reference codes and 3600 text bytes per
policy. Their PCM hashes and every ratio result agree with the unchanged
adaptive-edge report. Aggregate errors are retained below, rather than using
the new unequal-tone wins to conceal older failures.

| Candidate | Code edits | USOS-off text edits | USOS-on text edits |
| --- | ---: | ---: | ---: |
| Current ratio | 569 | 463 | 464 |
| Bounded normalization | 677 | 504 | 505 |
| Envelope offset | 616 | 472 | 476 |

Both corrections introduce errors at 5 dB AWGN, where the current detector
has none: seven raw code edits for bounded normalization, three for envelope
offset. Envelope offset also introduces four code edits on the common gain
step that the current detector recovers correctly. Per-run fields retain
exact substitutions/deletions/insertions and both rendered text views. These
regressions rule out replacing the current default with either candidate.

One-second silence/mark-only/space-only controls produce no observations for
any candidate at either sample rate or polarity. Five-second noise controls
for three seeds produce no printable characters and one CR per candidate
(12 kHz, mark-high, seed 7). The same noise waveforms are reused for both
polarities: thirty seconds of unique noise becomes sixty seconds of playback
per candidate, not sixty independent seconds. No multi-hour false-output gate
or calibrated squelch is implied.

Every unequal-tone run preserves observations, positions, counters, envelope
peaks, filter state, and final framing state across whole versus 127-sample
input. Additional one-sample, 20 ms, seeded random, and empty-call tests cover
both rates for strong imbalance, fading, and noisy imbalance. Reset clears
trained peaks and all frontend/framer history and matches a fresh trial.

## Prototype processing cost

Host: AMD Ryzen 9 9900X, Linux amd64, Go 1.26.2, default compiler settings,
no race instrumentation. Each case used one 500 ms Go benchmark run, replaying
a complete 2.893 s clean file through 20 ms chunks with preallocated results.
Fixture loading/construction are excluded; reset is included. These measure
the test-only wrapper, including the existing frontend's own framer and the
trial's additional framer. They are not estimates for an integrated receiver.

| Candidate | 12 kHz compute / audio file | 48 kHz compute / audio file | 12 / 48 kHz RTF |
| --- | ---: | ---: | ---: |
| Ratio wrapper | 1.226 ms | 4.943 ms | 0.000424 / 0.001708 |
| Bounded normalization | 1.377 ms | 5.493 ms | 0.000476 / 0.001899 |
| Envelope offset | 1.372 ms | 5.487 ms | 0.000474 / 0.001897 |

All cases report zero bytes and zero allocations per replay with sufficient
result capacity. This is a short balanced-signal cost comparison, not callback
percentiles, long-session resource qualification, or impaired-input benchmarks.

## Reproduction and decision

```sh
GOCACHE=/tmp/go-build task receiver:selective
GOCACHE=/tmp/go-build task bench:thresholds
GOCACHE=/tmp/go-build task check
# Export without overwriting retained evidence:
GOCACHE=/tmp/go-build go test ./internal/receiver -run '^TestSelectiveStudy$' \
  -count=1 -args -receiver-selective-output=/tmp/receiver-selective-new.json
```

The retained report is checked by ordinary tests; a passing test establishes
reproducibility, including the measured reception failures. Export refuses
existing files. Source/runtime hashes identify the tested implementation;
runtime version is provenance rather than an exact-version assertion.
`task check` passed ordinary and CGO-disabled suites, vet, race, version,
and the ten-test offline Python suite. Its optional resampler qualification
was skipped because the library was not configured. Calibration, exact ratio
replay, candidate reset/chunking checks, and the six benchmark cases passed.
The race-instrumented receiver suite took about 150 seconds on this host;
that duration is not an uninstrumented DSP performance measurement.

Keep the adaptive-edge ratio detector as the current implementation. The next
detector experiment needs noise-aware envelope estimation and a reliable
decision about when correction is justified, tested against this fixed
comparison. Physical frequency-selective channels, adjacent-signal rejection,
long idle/fade recovery, full noise-only hours, and scored HF comparison still
need independent evidence. No model or default changes are justified solely
by the zero-error envelope-offset result on the new development derivatives.
