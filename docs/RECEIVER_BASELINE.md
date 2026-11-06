<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Receiver experiment: clean baseline

Date: 6 November 2026. Implementation: [`internal/receiver`](../internal/receiver).
This is an internal experiment, not the public streaming receiver or completion
of the stage 3 HF reception gate. No native dependency was introduced.
The subsequent [controlled impairment study](RECEIVER_IMPAIRMENTS.md) preserves
this detector unchanged and records noise, gain-step, and dropout failures.
Those measurements describe the original one-bit detector. The later
[gain-step timing update](RECEIVER_GAIN_STEPS.md) documents the current
receiver's added timing path, preserved original evidence, and measured cost.

## Method

The baseline mixes real PCM with separate continuous complex mark/space
oscillators. Each branch uses a rectangular FIR window of one rounded bit
period (264 samples at 12 kHz, 1056 at 48 kHz for these fixtures). A ring and
running sum implement the window without recomputing a dot product. Oscillators
are normalized every 4096 input samples. No decimation is performed, so this
experiment introduces no decimation alias path or anti-alias qualification.
The rectangular window's sidelobes and susceptibility to interference remain
reception risks. More selective filters and ATC need measured comparisons.

The discriminator is `(Emark-Espace)/(Emark+Espace)`. A normalized summed-energy
floor of `1e-8` guards numerical silence; this is not the proposed noise-relative
squelch and is not wired to public `DecoderOptions`. Input has fixed division
by 32768, with no chunk normalization. The framer uses interpolated zero
crossings, one-bit acquisition mark, half-bit start checking, LSB-first data
samples, continuous stop checks from 6.25 to 6.75 bits, and a pending stop with
the design's 0.20-bit early-edge allowance. Earlier next starts must validate
before the preceding observation is emitted. Longer mark idle is accepted.
There is no AFC, adaptive clock recovery, carrier-loss policy, or synthetic
flush. Reset discards all filter, phase, frame, counter, and text state.

Observations retain raw codes, rendered text, estimated fractional source-sample
start/end, and the input sample where they became available. Estimates subtract
the rectangular FIR's nominal `(N-1)/2` group delay. They remain experimental:
transition bias depends on neighboring symbols and tones. NUL/BEL and shifts
produce empty text; SPACE, CR, and LF retain their specified rendering. USOS
is tested both off and on. These are framed observations, not verified messages.

## Independent clean audio

The unchanged [`independent.json`](../testdata/manifests/independent.json)
manifest SHA-256 is
`d6ea901c6f136a07d217d4d96451a2fa8388006512971ef7b59bc4eb9655a901`.
Its WAV hashes are checked before decoding. All audio is unmodified minimodem
0.24 output, using 909/20 baud, full-file scoring, and real lead/tail. No
expected text, codes, trimming, or fixture hashes were changed for the receiver.

All nine WAVs pass under each USOS policy: 142 raw codes and 100 rendered text
bytes per policy, with zero code/text differences and zero rejected candidates.
The two policies reuse the same audio; these are not 18 distinct recordings.
An additional test recovers the same codes/text using the default 500/11
receive baud against the 909/20-baud recordings. This small mismatch is not the
release's clock/baud stress qualification.

| Fixture | Codes | Text bytes | Max delay after known frame end, ms | Max absolute start estimate error, ms |
| --- | ---: | ---: | ---: | ---: |
| mixed-12000-mark-high-stop3 | 17 | 12 | 22.000 | 0.130 |
| mixed-12000-mark-high-stop4 | 17 | 12 | 22.083 | 0.130 |
| mixed-12000-mark-low-stop3 | 17 | 12 | 22.000 | 0.101 |
| mixed-12000-mark-low-stop4 | 17 | 12 | 22.000 | 0.092 |
| mixed-48000-mark-high-stop3 | 17 | 12 | 22.042 | 0.123 |
| mixed-48000-mark-high-stop4 | 17 | 12 | 22.062 | 0.123 |
| mixed-48000-mark-low-stop3 | 17 | 12 | 21.979 | 0.113 |
| mixed-48000-mark-low-stop4 | 17 | 12 | 22.021 | 0.103 |
| usos-12000-mark-high-stop3 | 6 | 4 | 22.000 | 0.066 |

Known boundaries use each fixture's independently recorded lead, rounded bit,
and stop lengths. Delay measures availability, excluding caller callback/device
latency. It reaches roughly one bit because early next-start validation can
add half a bit to the half-bit FIR delay. These short clean files do not
qualify the release's latency or reception targets.

## Streaming and rejection checks

All nine complete WAVs produce identical observations, including positions and
text, cumulative counters, and final filter/oscillator/framer state when fed
whole, in one-sample calls, 127-sample chunks, 20 ms callbacks, 1024-sample
chunks, or seeded random partitions. Empty calls are interleaved. Every one of
the 2905 split positions in a 2904-sample real prefix is checked separately.
Reset is exercised at four input positions per fixture, with parallel independent
instances. Truncation before a stop emits no invented final character; later
real audio resumes the pending frame.

Separate logical timing tests bypass the detector: all 32 CSV wire codes,
both stop selections, earliest permitted next starts, nominal/late starts,
longer idle, stops below the allowed bound, one-stop senders, invalid stop
space, acquisition mark just below/above one bit, and failed early-start
validation. These isolate framer behavior; they do not qualify the same bounds
through impaired audio. Every letters/figures rendering entry is checked against
the independent CSV. One second each of silence, mark-only, and space-only
PCM produces no observations at each sample rate and polarity. Noise-only
false-output hours remain unperformed.

## Processing cost

Host: AMD Ryzen 9 9900X 12-Core Processor; Linux
7.2.15-200.fc44.x86_64; Go 1.26.2 linux/amd64, GOAMD64=v1, CGO_ENABLED=1.
Backend is exclusively Go; default compiler settings, no race instrumentation.
Benchmarks used 20 ms chunks and preallocated output, with reset and complete
fixture processing on each iteration. Fixture loading, hashing, construction,
and PCM allocation are excluded. Three one-second benchmark repetitions:

| Clean mark-high, 1.5 stop | Audio duration | ns/op range | RTF range | Allocations |
| --- | ---: | ---: | ---: | ---: |
| 12 kHz | 2.893 s | 280951–281873 | 0.0000971–0.0000974 | 0 B/op, 0 allocs/op |
| 48 kHz | 2.893 s | 1113553–1116347 | 0.0003849–0.0003859 | 0 B/op, 0 allocs/op |

RTF is compute time divided by audio duration. Each repetition reports an
average over many complete replays, not callback p95/p99 or a long-session
latency distribution. Persistent correlation rings occupy 8448 bytes at 12 kHz
and 33792 bytes at 48 kHz; result storage is separate. Long-session memory,
numeric drift, other CPUs, noise/impaired performance, and callback latency
percentiles have not been qualified.

## Reproduction and decision

```sh
GOCACHE=/tmp/go-build task receiver:baseline
GOCACHE=/tmp/go-build go test ./internal/receiver
GOCACHE=/tmp/go-build task bench:receiver
GOCACHE=/tmp/go-build task check
```

The ordinary, CGO-disabled, vet, race, version, and offline Python checks pass.
The Python resampler qualification is explicitly skipped when its library is
not configured. A bounded-PCM fuzz run with two workers passed a ten-second
budget (412 executions); this is a short development check, not sustained
fuzzing evidence. The PCM loader validates WAV structure before allocation and
shares the existing strict fixture parser. WAV fuzz seeds now check extraction
as well as inspection.

Continue Go receiver experiments. These clean fixtures establish acquisition,
framing, and streaming feasibility, with no measured need for native processing.
They do not resolve receiver quality. Before advancing to the production API:
admit trusted aligned HF transcripts and freeze development/held-out groups;
compare pinned fldigi on scored ranges; extend the short controlled
noise/tuning/clock/gain-step measurements to selective fading and interference;
and evaluate stronger filters/ATC from those failures. HF corpus admission,
fldigi-generated TX fixtures, impaired/audio
timing gates, false-output hours, and the Go-versus-extraction quality decision
remain open. External decoders were not rerun for this change; the input evidence
is the existing pinned independent WAVs and captures.
