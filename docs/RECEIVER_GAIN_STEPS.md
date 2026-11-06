<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: amplitude-step timing

Date: 6 November 2026. This is a focused internal receiver change following
the [controlled impairment study](RECEIVER_IMPAIRMENTS.md), not a public modem
API or completion of the HF reception gate.
The later [unequal-tone threshold study](RECEIVER_SELECTIVE.md) compares
correction candidates against this receiver; measured regressions keep those
candidates test-only and leave this implementation unchanged.

## Failure and change

All three missing codes in the original common 20 dB step case came from the
short USOS probe. Its expected codes are `[31,3,27,23,4,3]`; the original
receiver emitted `[31,3,3]`. A gain drop about 40 samples after the FIGS start
biased the one-bit rectangular detector's start estimate roughly 85 samples
late (0.32 bit at 12 kHz). The next real start then appeared before the
pending frame's 7.30-bit earliest permitted edge and was rejected. Filter
memory weighted older high-amplitude audio more heavily than newer weak audio;
the ratio discriminator alone could not remove that timing bias.

The current receiver retains the one-bit correlators, energy floor, and full
data/start/stop decisions. It adds a separately bounded timing path:

1. Two half-bit rectangular correlators run continuously alongside the
   original mark/space branches. Their metric is delayed by 66 samples at
   12 kHz or 264 at 48 kHz, aligning its nominal source timeline with the
   one-bit path. Sample positions still compensate the one-bit group delay.
2. A one-bit raw mean-square ring compares its recent and older half-bit
   power sums. After a full window exists, either sum below one quarter of
   the other identifies at least a 6 dB broadband power disparity. The full
   window's mean power must exceed `1e-8`. This is a gain-transient heuristic,
   not carrier squelch, a correctness probability, or selective-fading ATC.
3. For one bit after the last qualifying disparity, falling-edge detection,
   acquisition mark qualification, and pending-stop handoff use the aligned
   short metric. Otherwise they use the original long metric. The start
   half-bit check, five data decisions, and 6.25–6.75-bit stop check always
   use the original long metric. No framing tolerance is widened.
4. Only the short timing path bounds instantaneous input to
   `±4*sqrt(max(long_mean_square,1e-8))`. This limits isolated impulses that
   otherwise also trigger the broadband power heuristic. Original samples
   still feed the long branches and energy gate; no input PCM is modified.

All rings, oscillator phases, power sums, delay position, and transient hold
advance on input samples. Empty calls change nothing; Reset clears every new
state component. No samples are synthesized, and the library adds no device,
radio, or native backend dependency. The unchanged one-bit detector remains
available internally for reproducing the original evidence.

## Retained comparison

The original [`receiver-channel-v1.json`](../testdata/reference/receiver-channel-v1.json)
is unchanged. The new
[`receiver-adaptive-edge-v2.json`](../testdata/reference/receiver-adaptive-edge-v2.json)
retains the same 324 generated inputs, with identical PCM hashes, lengths,
clipping, and scoring expectations. It adds the receiver identity and source
hashes. The current harness checks the new report and rejects any per-run
increase in code or either text edit distance against the original baseline.
The original detector can also reproduce the original input/output fields.
Only historical harness-source/runtime/receiver metadata are exempt from that
legacy comparison: the historical source hash is preserved, not claimed to
match the expanded comparison harness.

| Retained condition | Original | Current |
| --- | ---: | ---: |
| Common step, raw code edits / 142 | 3 | 0 |
| Common step, USOS-off text edits / 100 | 3 | 0 |
| Common step, USOS-on text edits / 100 | 2 | 0 |
| Other 323 signal runs | Original outcomes | Identical retained outcomes |
| Six short noise-only runs | Zero printable, one CR | Identical codes/text/counters |

Only the USOS probe's common-step result changes: its six codes, required
shift, space, and both text views are recovered exactly, with no rejected
candidates. The weak-noise and dropout failures remain visible. Timing
coordinates during amplitude changes may differ intentionally; steady-gain
clean observations and counters are explicitly identical to the original
receiver, and each current stream is invariant to PCM chunking.

Additional development tests apply 20 dB steps to all nine independent WAVs,
with starts at 31/35/39% and ends at 61/65/69% of each full file. Boundaries use
integer floor division. Fixed gain is 1/16, then the faded interval has another
factor of 0.1 before round-to-even PCM conversion. These 81 source/position
cases compare 1278 expected codes, and 900 expected text bytes per USOS policy.
The original detector has 13 code edits in five cases; the current detector
recovers all codes and both text views exactly. These positions were selected
as development regressions after the initial failure was observed; they are
not held-out evidence or a general gain-step guarantee.

Whole/20 ms/127-sample invariance is checked throughout the full impairment
study. One-sample/random/empty-call checks include the common step as well as
noise, dropout, and tuning/clock cases. Reset is explicitly exercised during
an active transient hold and compared with a fresh receiver. Ordinary clean,
logical framing/rejection, silence/steady-tone, instance-isolation, and default
baud tests remain applicable.

## Cost and reproduction

On the clean-baseline Ryzen 9 9900X / Linux amd64 / Go 1.26.2 host, three
one-second clean benchmark repetitions with 20 ms chunks measured about
0.77 ms per 2.893 s file at 12 kHz and 3.05 ms at 48 kHz. RTF was roughly
0.00027 and 0.00106 respectively, with zero bytes/allocations per replay when
result capacity is provided. This is approximately 2.7 times the original
compute cost. These short averages do not qualify callback percentiles or
long-session latency. Persistent ring storage rises from 8448 to 15312 bytes
at 12 kHz and from 33792 to 61248 at 48 kHz, excluding instance/result storage.

```sh
GOCACHE=/tmp/go-build task receiver:impairments
GOCACHE=/tmp/go-build task receiver:impairments:legacy
GOCACHE=/tmp/go-build go test ./internal/receiver \
  -run 'TestAmplitudeStepPositions|TestResetDuringAmplitudeStep|TestAdaptiveQuietPath' -v
GOCACHE=/tmp/go-build task bench:receiver
GOCACHE=/tmp/go-build task check
```

`task check` passed ordinary and CGO-disabled suites, vet, race, version, and
the offline Python suite (ten tests, optional resampler qualification skipped
because its library was not configured). The legacy comparison and targeted
gain-step/reset tests also passed. No external fixture or reference-tool
check is implied by these local results.

The new report can be exported with the existing `-receiver-study-output`
flag to a path that does not exist. Add `-receiver-study-legacy` to export the
original detector with current provenance. Original fixtures and the channel
manifest remain unchanged. External reference applications were not replayed;
the tests use their existing independent WAV evidence. Trusted HF scoring,
selective fading/interference, long-session numerics/resources, calibrated
squelch, and multi-hour false-output gates remain open. The next reception
work should use these retained failures and labelled HF comparisons to choose
stronger detector/ATC or recovery changes.
