<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: fade diagnosis and causal two-path channels

Date: 6 November 2026. This extends the [guarded-envelope experiment](RECEIVER_NOISE_AWARE.md).
It explains the remaining controlled-fade errors and adds a frequency-selective
convolution model. The current receiver and previous reports are unchanged.
No candidate is promoted: faster level tracking introduces regressions, and
deep/noisy two-path reception remains poor.

The [channel manifest](../testdata/manifests/receiver-two-path-v1.json) fixes
nine conditions and three detector candidates before measurement. A separate
[tracking manifest](../testdata/manifests/receiver-fade-tracking-v1.json) fixes
the new detector's single added time constant. The
[retained report](../testdata/reference/receiver-two-path-v1.json) records
810 signal outcomes and 24 controls: 459 prior inputs for the tracking
candidate, plus 117 two-path inputs for each of three candidates. It retains
source/input hashes, complete raw codes, both text policies, edit counts,
clipping, rejected frames, and guard/gate evidence. Inputs remain development
derivatives of the same nine independent minimodem WAVs; they do not add
independent transmitters or held-out/HF recordings.

## What the twelve fade errors contain

The frozen short-evidence detector loses one SPACE in each of the eight mixed
recordings under mark fading. On the short USOS probe, mark fading produces
one accepted wrong code, and space fading loses the final three codes.

The trace test records three representative failure mechanisms:

| Input | Observation |
| --- | --- |
| 12 kHz mark-high mixed, mark fade | SPACE's stop is rejected at input sample 14162. Correction first engages at 16495, after the loss. The old mark peak remains too high for the imbalance gate to engage in time. |
| USOS probe, mark fade | No correction and no framing rejection. Expected code 23 is received as 22: a framed but wrong figures character. |
| USOS probe, space fade | The first rejected start is at sample 6772; correction never engages. Output stops after `[31,3,27]`. |

These establish that not every fade failure is a gate handoff or malformed
stop. A valid frame is not verification of the transmitted character. The
trace uses actual independent PCM and event order; it does not replace missing
characters from expected text.

## Qualified falling-level candidate

The new `qualified-fall` trial retains half-bit evidence, guard positions,
noise subtraction, hysteresis, and all framing tolerances. It changes only
falling level estimates while the corresponding branch has maintained the
existing qualified dominance/noise evidence. Such an estimate approaches its
current observed energy with a one-bit time constant:

```text
peak += (1-exp(-1/T_samples)) * (observed_energy-peak)
```

Immediate eligible attack remains unchanged. During an unqualified/absent
branch, the existing sixteen-bit peak release remains. This tests whether
stale peaks can be repaired without learning directly from absence or noise.
The one-bit coefficient was fixed before measurement, not tuned to the results.

| Prior development matrix | Previous guarded-short code/text off/on edits | Qualified-fall edits |
| --- | ---: | ---: |
| Unequal-tone inputs | 12 / 12 / 12 | 8 / 8 / 8 |
| Earlier impairment inputs | 569 / 463 / 464 | 591 / 481 / 483 |

It corrects the short probe's four fade errors, but the eight mixed-message
SPACE losses remain. It also introduces 22 code edits on the common amplitude
step that the prior detector recovers correctly. Independent branch-level
adjustment can temporarily look like an imbalance during a common gain change.
The result rejects this candidate as a replacement; the smaller fade total
does not justify hiding the common-step regression.

## The two-path model

Unlike the earlier symbol-conditioned amplitude model, this generator never
receives bit labels, expected codes, or receiver decisions. It applies delayed
copies directly to the independently generated real PCM:

```text
y[n] = G * (x[n] + a[n]*x[n-D]) / (1+a_max) + noise[n]
```

`G=1/16`. Input is zero outside its stored support; output has `N+D` samples,
retaining the entire causal delayed response. This is modelled channel
response, not receiver flush padding. Fixed normalization by `1+a_max` bounds
the signal's absolute path gains without per-chunk or post-channel AGC.
Both original PCM and its phase are preserved before this linear operation.

Delays use integer ticks of 1/12000 s, scaled exactly for 12 or 48 kHz:

- 34 ticks (2.8333 ms) attenuate the higher 1585 Hz tone.
- 38 ticks (3.1667 ms) attenuate the lower 1415 Hz tone.

Tone labels are physical audio frequencies, independent of logical mark/space
polarity. Static echo amplitudes are 0.5 and 0.95. Fading conditions vary echo
amplitude smoothly from zero to 0.95 and back over the middle half of the
source file, using `(1-cos(2*pi*u))/2`, with
`u=(n/N-0.25)/0.5`. Gain is evaluated at output time `n`.

Noisy cases use amplitude 0.9 and three fixed seeds, with Gaussian noise added
after convolution. SNR is 10 dB relative to the unfaded, scaled source power
`P` in 2500 Hz: `sigma^2=P*Fs/(2*2500*10^(10/10))`. It is not the local SNR
inside a notch. PCG uses `(seed,0x74776f70617468)` and `NormFloat64`. Conversion
is round-to-even of `32768*y`, with counted saturation. No two-path derivative
clipped. Existing clipped prior inputs retain their original clipping counts.

This is a controlled physical mechanism, not a measured HF channel. It lacks
complex RF path phase/doppler statistics, an independently measured delay
profile, adjacent signals, and real station noise. Its frequency-selective
response and intersymbol memory are nevertheless obtained from waveform
convolution rather than transmitter-state-dependent gain.

## Channel calibration

For a static echo, normalized amplitude response is
`|1+a*exp(-j*2*pi*f*D/Fs)|/(1+a)`. Steady PCM tone tests at 1415/1585 Hz,
both rates, both delays, and both echo amplitudes agree with this analytic
prediction within 0.1 dB. Representative normalized attenuations are:

| Delay | Echo amplitude | 1415 Hz | 1585 Hz |
| --- | ---: | ---: | ---: |
| 34 ticks | 0.5 | −0.003 dB | −9.514 dB |
| 34 ticks | 0.95 | −0.004 dB | −28.280 dB |
| 38 ticks | 0.5 | −9.418 dB | −0.014 dB |
| 38 ticks | 0.95 | −23.689 dB | −0.016 dB |

A separate literal impulse vector checks direct/delayed tap positions,
normalization, output length, and PCM rounding. Identity is exact on every
independent WAV and output storage does not alias the source.

The deep notches are stronger than the earlier model's 20 dB attenuation.
These results do not isolate dispersion from notch depth or local SNR. Matched
depth and separate phase/delay sweeps remain useful follow-up comparisons.

## Receiver comparison

All original expected codes and text remain unchanged. Complete output over
the `N+D` channel response is scored, including any extra observations. There
is no trimming, line-ending cleanup, or error correction. Deterministic cases
compare 142 reference codes and 100 text bytes per policy; noisy cases compare
426 codes and 300 bytes across three seeds.

| Two-path condition | Current ratio code edits | Previous guarded-short edits | Qualified-fall edits |
| --- | ---: | ---: | ---: |
| Identity | 0 | 0 | 0 |
| High-tone notch, moderate | 65 | 0 | 0 |
| Low-tone notch, moderate | 50 | 0 | 0 |
| High-tone notch, deep | 138 | 70 | 70 |
| Low-tone notch, deep | 138 | 86 | 90 |
| High-tone notch, fading | 34 | 0 | 4 |
| Low-tone notch, fading | 36 | 1 | 4 |
| High-tone notch, noisy | 408 | 319 | 328 |
| Low-tone notch, noisy | 411 | 283 | 294 |
| **Total / 1846 reference codes** | **1280** | **759** | **790** |

Aggregate text edits (USOS off/on) are 943/942 for the current ratio,
596/594 for guarded-short, and 629/627 for qualified-fall. Raw-code and text
scoring are retained separately because missing shifts affect subsequent text.

Guarded-short recovers the moderate cases and most noiseless fading input,
but does poorly at deep notches and noisy echoes. Qualified falling-level
tracking is worse overall here as well as on the common-step regression.
Neither this model nor the prior amplitude model qualifies a release receiver.

Silence/steady-tone controls remain silent. The tracking candidate retains
the baseline's short noise outcome: zero printable characters and one CR
(12 kHz, mark-high, seed 7). Short controls are not the required false-output
hours and do not establish calibrated squelch.

## Streaming, cost, and validation

Whole versus 127-sample input preserves every observation, position, counter,
filter/guard/evidence state, and final framing state for all 810 signal results,
including failed reception. Additional one-sample, 20 ms, random, empty-call,
and trained-reset checks exercise both rates for high-notch deep/fading/noisy
inputs and all candidates. USOS does not change recovered raw codes.

No source-frame latency is reported for the physical channel: propagation,
phase, and intersymbol response have not been separately aligned/qualified.
Prior development inputs retain their existing conditional delay scoring.

On the Ryzen 9 9900X / Linux amd64 / Go 1.26.2 host, one 500 ms benchmark
per rate/candidate replays a clean 2.893 s source file with 20 ms chunks:

| Candidate | 12 kHz compute | 48 kHz compute | RTF, 12 / 48 kHz |
| --- | ---: | ---: | ---: |
| Current ratio receiver | 0.788 ms | 3.035 ms | 0.000272 / 0.001049 |
| Guarded-short prototype | 1.635 ms | 6.390 ms | 0.000565 / 0.002209 |
| Qualified-fall prototype | 1.724 ms | 6.670 ms | 0.000596 / 0.002305 |

All report zero bytes/allocations per replay with preallocated results.
Construction, WAV loading, and channel generation are excluded; reset is
included. Guarded prototypes include the shared frontend's own framer plus
their trial framer, unlike the direct ratio implementation. These are short
clean-processing measurements, not integrated performance, callback
percentiles, or long-session memory evidence.

```sh
GOCACHE=/tmp/go-build task receiver:paths
GOCACHE=/tmp/go-build task test:race-paths
GOCACHE=/tmp/go-build task bench:paths
GOCACHE=/tmp/go-build go test ./...
CGO_ENABLED=0 GOCACHE=/tmp/go-build go test ./...
GOCACHE=/tmp/go-build go vet ./...
# Export to a path that does not exist:
GOCACHE=/tmp/go-build go test ./internal/receiver -run '^TestPathStudy$' \
  -count=1 -args -receiver-path-output=/tmp/receiver-path-new.json
```

Full ordinary and CGO-disabled suites, vet, and targeted race checks for every
new study/calibration/streaming/trace test passed. The offline Python suite
passed ten tests with optional resampler qualification skipped. The full
historical race matrix was not rerun for this test-only addition; the targeted
race command exercised all new processing paths, taking about 109 seconds.
External fldigi/minimodem tools and real HF recordings were not replayed.

Retain the current receiver and the earlier guarded-short candidate. Do not
adopt independently accelerated peak tracking on this evidence. The next
detector design needs to distinguish common level changes from changing
branch imbalance, then establish deep-start acquisition and rejection of
noise/interference on matched physical-channel cases and trusted HF labels.
