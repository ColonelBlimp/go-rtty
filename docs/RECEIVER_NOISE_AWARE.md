<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: guarded envelope correction

Date: 6 November 2026. This follows the [unequal-tone comparison](RECEIVER_SELECTIVE.md).
The strongest new candidate reduces that model's code edits from 1204 to 12,
with no per-run code/text regression on the 324 previous impairment inputs.
It remains test-only: physical frequency-selective channels, scored HF audio,
and the full resource/false-output gates remain unqualified. The current
adaptive-edge ratio receiver and all previous reports are unchanged.
The later [two-path channel study](RECEIVER_TWO_PATH.md) traces these residual
fade errors, rejects a faster level tracker with common-step regressions, and
measures the guarded candidate's limits on waveform convolution channels.

## Evidence and experimental sequence

The first [parameter manifest](../testdata/manifests/receiver-noise-aware-v1.json)
fixes two guarded candidates before measurement. Its
[report](../testdata/reference/receiver-noise-aware-v1.json) retains 918 signal
results and 48 negative-control results. These reuse the same 135 unequal-tone
inputs, 324 previous impairment inputs, and 24 control playbacks per candidate.
No waveform, transcript, expected code, scoring interval, or earlier report
was altered to improve the result.

That experiment recovered the earlier balanced-noise/gain-step behavior, but
its long-window evidence acquired isolated weak spaces too late. A separate
[follow-up manifest](../testdata/manifests/receiver-noise-short-v1.json) and
[report](../testdata/reference/receiver-noise-short-v1.json) retain the half-bit
evidence variant: 459 signal outcomes and 24 controls. The first report and
its implementation remain intact and reproducible. The follow-up changes
evidence bandwidth, not channel conditions or the earlier frozen constants.
Its selection follows inspection of a development failure; it is not held-out
validation or a new independent transmitter.

Both reports record input/source hashes, actual raw codes and both text views,
separate edit counts, clipping, framing rejections by state, correction and
qualification sample counts, gate activations, first correction position,
guard-energy sums, and eligible frame-delay measurements. All waveform hashes
are checked against the retained input evidence before decoding. USOS-on text
is rendered from the observed raw codes; separate streaming tests also run
actual USOS-on instances.

## Guard measurement and evidence gate

The test-only prototype adds equal-length complex rectangular correlators at
`center ± (shift/2 + 340 Hz)`. The supported development tuning therefore uses
1075/1925 Hz guards around 1415/1585 Hz data tones. Guards are outside both
data branches. The weaker data branch is never substituted for noise.
Both guard windows have the one-bit data window's length and normalization.
Their mean energy is smoothed causally over two bit periods, with a numerical
floor of `1e-8`. This is a noise-plus-leakage estimate, not calibrated squelch.

The experimental constructor rejects a tuning whose guard frequencies leave
the checked audio passband. This is an experiment restriction, not a new
restriction on public `Config` or the current receiver. No decimation, new
native dependency, audio device, CAT, or radio operation is introduced.

Independent guard calibration uses real white noise with sigma 0.03. Expected
normalized complex-correlator energy is `sigma^2/N` for `N` real input samples:

| Rate | Expected mean | Measured mean |
| --- | ---: | ---: |
| 12 kHz | 3.4091e-6 | 3.3273e-6 |
| 48 kHz | 8.5227e-7 | 8.2210e-7 |

These finite runs pass a 20% calibration tolerance. Pure mark/space tests
measure mean guard leakage about 0.00107–0.00108 of the desired tone's
energy. Thus the guard estimate includes carrier sidelobe leakage even with
no noise. This measurement does not qualify its bias under adjacent RTTY,
other tuning, real fading, or broadband interference.

The fixed gate uses these sample-clock rules:

1. Track each branch's energy peak with immediate eligible attack and sixteen
   bit periods of release. Update a peak from a branch only while that branch
   exceeds four times guard energy and twice the opposite branch's energy.
2. Require that branch evidence continuously for a quarter bit before recording
   recent qualification. Both branches must have qualified within sixteen
   bits, and the smaller peak must remain above four times guard energy.
3. Engage correction when the larger peak is at least four times the smaller.
   Remain engaged until that ratio falls to two or the evidence/noise checks
   fail. Otherwise feed the original ratio metric directly to the framer.
4. Subtract the guard estimate, clamped at zero, before the envelope-offset or
   bounded-normalization calculation. Normalization also retains the previous
   1/16 relative floor. These are heuristics, not probabilities of correctness.

Correction applies to the selected timing metric as well as long-window
decisions. Raw energy gating, start/data/stop checks, early-edge tolerances,
the existing impulse bound, and real-sample-only processing remain unchanged.
All guards, evidence ages, peaks, hold state, delay rings, and counters persist
across chunks and are cleared by Reset.

## Why short evidence matters

On the 12 kHz mark-high mixed recording with space 20 dB down, the first real
start begins at sample 528, or two bit periods. Long-window guarded offset
first corrects at sample 3606, about 13.66 bits into the recording. The trace
contains three rejected starts and two rejected stops. Early incorrect frames
cannot be repaired by enabling correction later.

A one-bit correlator mixing older strong mark with a single weak-space bit
does not provide a quarter bit of sufficiently dominant weak-space evidence.
The later longer space run supplies it, explaining the observed late first
qualification. This is a measured acquisition problem, not a reason to
replace failed characters or infer them from the expected transcript.

The follow-up uses the existing half-bit correlators for peak/evidence
qualification while retaining one-bit data decisions. A half-length window
has twice the white-noise energy after normalization. Its evidence energies
are divided by two before applying the same guard/evidence rules. For
correction, peak estimates are restored to signal units and twice the long
guard estimate is removed from those peaks; long data energy still removes
one guard estimate. A separate white-noise test measures half/whole energy
ratios 2.016 at 12 kHz and 1.882 at 48 kHz, consistent with two within the
fixed 20% tolerance.

This variant first corrects at sample 708, or 2.68 bits, while the first real
start bit is still arriving. It recovers the full six-code USOS probe and
all seventeen mixed-message codes with 20 dB weak space. The isolated-start
regression covers all nine independent WAVs under both USOS policies. The
follow-up has no transmitter-code input or retrospective text repair.

## Results and remaining failures

The unequal-tone matrix compares 2130 reference codes and 1500 text bytes per
policy per candidate. The earlier impairment matrix compares 5112 codes and
3600 bytes per policy. These are repeated development derivatives of nine
source WAVs, not additional independent recordings.

| Candidate | Unequal-tone code edits | Unequal-tone text edits off/on | Prior-matrix code edits | Prior text edits off/on |
| --- | ---: | ---: | ---: | ---: |
| Current ratio | 1204 | 942 / 946 | 569 | 463 / 464 |
| Ungated envelope offset (earlier study) | 0 | 0 / 0 | 616 | 472 / 476 |
| Guarded offset, long evidence | 153 | 120 / 120 | 569 | 463 / 464 |
| Guarded normalization, long evidence | 391 | 313 / 311 | 568 | 462 / 463 |
| Guarded offset, short evidence | 12 | 12 / 12 | 569 | 463 / 464 |

The long guarded offset removes the earlier 5 dB AWGN and common-step
regressions. It still worsens two short USOS/noisy-space cases relative to
the current ratio. Guarded normalization worsens numerous weak-space cases.
The short-evidence variant improves 106 of 135 unequal-tone runs, leaves
the other 29 code/text outcomes unchanged, and has no per-run increase in
code or either text edit distance. All 324 prior code arrays and both text
views remain identical to the current receiver; framing rejection counts
can differ without changing emitted observations' code/text sequences.

Twelve code/text edits remain: nine in the controlled mark-fade condition and
three in the space-fade condition. Every static 6/12/20 dB attenuation case
and both 12 dB imbalance/10 dB noise conditions has zero code/text edits with
short evidence. These results do not establish a physical fading operating
limit. Peak dynamics and gate handoff during a changing fade remain a target
for investigation.

Silence and steady mark/space produce no observations in the controls. All
three new variants retain the current short noise-only outcome: zero
printable characters and one CR, with zero correction samples. The same
noise waveforms are replayed under both polarities; this still represents
thirty seconds of unique noise, not multi-hour false-output evidence.

Whole/127-sample processing preserves every observation, sample coordinate,
counter, noise estimate, peak, and final state throughout both new studies,
including failed reception. Additional one-sample, 20 ms, random, empty-call,
and trained-state reset checks cover both sample rates. Balanced clean audio
matches the current receiver's observations, counters, and framing state with
correction disabled. The half-bit follow-up has its own reset/chunk checks.

Delay is retained only for complete matching code sequences with starts
within half a bit of their known indexed frames. Eligible maxima for short
evidence are about 23.9 ms after known frame end on unequal-tone inputs and
24.9 ms on the prior matrix. This excludes device/callback delay and is not
a latency-percentile qualification. Errors and unaligned runs have no delay
value; they are not reassigned to nearby frames to obtain a latency number.

## Cost, reproduction, and decision

On the same Ryzen 9 9900X / Linux amd64 / Go 1.26.2 host, one 500 ms benchmark
per clean rate/candidate measured these complete 2.893 s file replays with
20 ms chunks and preallocated output:

| Candidate | 12 kHz compute | 48 kHz compute | RTF, 12 / 48 kHz |
| --- | ---: | ---: | ---: |
| Guarded offset, long evidence | 1.615 ms | 6.451 ms | 0.000558 / 0.002230 |
| Guarded normalization, long evidence | 1.609 ms | 6.282 ms | 0.000556 / 0.002171 |
| Guarded offset, short evidence | 1.602 ms | 6.449 ms | 0.000554 / 0.002229 |

All report zero bytes/allocations per replay. These test-only prototypes
include the shared frontend's framer and an additional trial framer; they
are not integrated receiver performance or long-session memory results.

```sh
GOCACHE=/tmp/go-build task receiver:noise
GOCACHE=/tmp/go-build task bench:noise
GOCACHE=/tmp/go-build task check
# Each export path must not already exist:
GOCACHE=/tmp/go-build go test ./internal/receiver -run '^TestNoiseAwareStudy$' \
  -count=1 -args -receiver-noise-output=/tmp/receiver-noise-new.json
GOCACHE=/tmp/go-build go test ./internal/receiver -run '^TestNoiseShortEvidenceStudy$' \
  -count=1 -args -receiver-noise-short-output=/tmp/receiver-noise-short-new.json
```

`task check` passed ordinary and CGO-disabled suites, vet, race, version, and
the ten-test offline Python suite. Optional resampler qualification was skipped
because its library was not configured. Guard/noise-bandwidth calibration,
balanced fallback, weak-start acquisition, trained reset, and partition tests
passed. All six prototype benchmark cases completed with preallocated output
and no steady-state allocations. The complete race-instrumented receiver suite
took about 317 seconds; this is validation cost, not an uninstrumented DSP
throughput measurement.

The reports preserve failed reception and historical evidence. A passing
report test establishes reproduction and streaming invariants, not release
acceptance. No external reference application or off-air recording was
replayed for this change. The next experiments should inspect the remaining
fade/gate handoffs and test a physical frequency-selective model, adjacent
interference, and independently labelled HF audio. Retain the current default
until that evidence and wider guard-placement/tuning coverage justify adoption.
