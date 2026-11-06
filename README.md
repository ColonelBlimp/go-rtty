# go-rtty

`go-rtty` is a project to build a standalone Go library for RTTY reception,
text and protocol encoding, and audio frequency-shift keying (AFSK) generation.
It is intended for applications that need a streaming modem while retaining
control of their audio devices and station operation.

## Status

The project has its initial reference and development foundation: pinned
external tools, independent protocol/audio fixtures, artifact validation tests,
Task commands, and a Linux CI workflow. The public [`rtty`](rtty/doc.go) package
now provides US teleprinter text encoding, protocol framing, configuration,
audio-tone calculations, option validation, and typed errors. PCM reception,
AFSK generation, and encode/decode command-line tools remain unimplemented in
the public API. An [internal receiver experiment](docs/RECEIVER_BASELINE.md)
now decodes the nine independent clean WAV fixtures with zero code/text errors
and passes PCM chunk-invariance checks. HF reception remains unqualified.
The [controlled impairment study](docs/RECEIVER_IMPAIRMENTS.md) records tuning
and clock tolerance on these short files, plus noise, gain-step, and dropout
failures. It provides a reproducible development baseline rather than a
reception acceptance result.
The [gain-step timing update](docs/RECEIVER_GAIN_STEPS.md) now recovers the
observed common-step losses with no per-run code/text regression across that
study. Noise and undeclared-dropout failures remain open.
The [unequal-tone threshold comparison](docs/RECEIVER_SELECTIVE.md) now records
large gains from two experimental envelope corrections, alongside regressions
on the existing noise/gain-step cases. Both candidates remain test-only; the
current receiver is unchanged.
The later [guarded-envelope follow-up](docs/RECEIVER_NOISE_AWARE.md) reduces
controlled unequal-tone code errors from 1204 to 12 while preserving all 324
prior code/text results. It remains test-only pending physical-channel, wider
tuning, and labelled-HF validation.
The [two-path channel study](docs/RECEIVER_TWO_PATH.md) now calibrates a causal
frequency-selective convolution against its analytic response. It retains
deep/noisy echo losses and rejects a faster level tracker that worsens common
gain steps. The current receiver and prior guarded candidate are unchanged.

The ordinary tests validate text encoding, framing, configuration contracts,
the experimental receiver on clean/impaired development audio, and retained
reference evidence. See the
[reference recipe and validation record](docs/REFERENCE.md) and
[corpus inventory](docs/CORPUS.md) for what has actually been exercised and
what remains unperformed.

An explicit [external corpus workflow](docs/REFERENCE.md#external-gritty-development-workflow)
fetches five checksum-pinned GRITTY recordings and
[three W6IWI HF tests](docs/REFERENCE.md#external-w6iwi-development-workflow),
prepares fixed 12/48 kHz derivatives, and supports unscored fldigi and minimodem
captures. Audio and captures remain in a caller-selected external cache.
The [transcript review workflow](docs/TRANSCRIPTS.md) adds original-rate review
packets, explicit admission, matched Go/fldigi capture plans, and printable-view
development scoring with optional known-code comparisons. It requires
independent reviewed truth; draft labels cannot be scored.
Trusted aligned transcripts and held-out reception validation are outstanding.

The [design document](docs/RTTY_DESIGN.md) specifies the proposed architecture,
streaming contracts, receive and transmit processing, validation requirements,
performance targets, and delivery stages. Its implementation status identifies
the available API subset; modem examples and acceptance targets describe
planned behavior.

## Configuration

Start with explicit defaults, then set the caller's audio tuning and polarity:

```go
cfg := rtty.DefaultConfig()
cfg.CenterHz = 2210
cfg.Polarity = rtty.MarkLow
markHz, spaceHz, err := cfg.ToneFrequencies() // 2125 Hz mark, 2295 Hz space
if err != nil {
    return err
}
fmt.Printf("mark %.0f Hz, space %.0f Hz\n", markHz, spaceHz)

opts := rtty.DefaultDecoderOptions()
opts.Squelch.Enabled = false // Explicitly request ungated reception.
if err := opts.Validate(); err != nil {
    return err
}
```

Import `github.com/ColonelBlimp/go-rtty/rtty` (and `fmt` for the output above).
`Config.Validate` and
`ToneFrequencies` reject unsupported profiles and invalid enums with
`ErrInvalidConfig` and `*ConfigError`. `DecoderOptions.Validate` returns
`ErrInvalidOptions` and `*OptionsError`. Typed errors expose `Field` and `Reason`
and support `errors.Is`/`errors.As`. See the [executable examples](rtty/example_test.go).

Zero `Config`, `Polarity`, `StopBits`, and `Alphabet` values are invalid.
Zero `DecoderOptions` explicitly disables USOS and squelch; the default options
enable squelch with candidate thresholds awaiting receiver evaluation. These
checks validate settings only; no decoder is created by this API subset.

## Text encoding

Encode a complete message with an explicit alphabet:

```go
encoded, err := rtty.EncodeText("a1a", rtty.TextOptions{Alphabet: rtty.AlphabetUS})
if err != nil {
    return err
}
fmt.Println(encoded.Text)  // A1A
fmt.Println(encoded.Codes) // [31 3 27 23 31 3]
```

The result includes the initial LTRS and all required letters/figures shifts.
SPACE preserves the transmitter's shift state and forces another FIGS before
the next figures character, for receivers with either USOS policy. CR, LF,
NUL, and BEL are preserved exactly; there is no line-ending conversion or
replacement of unsupported characters. ASCII lowercase becomes uppercase.

Invalid options return `ErrInvalidOptions` first. Malformed UTF-8 returns
`ErrInvalidInput` with `*InvalidUTF8Error`; otherwise an unsupported rune
returns `ErrUnsupportedText` with `*UnsupportedTextError`. Both text error
types expose the original byte offset, and the latter also exposes the rune.
Every error returns an empty result. Empty valid input emits no codes.

Each call is independent and produces caller-owned code storage. `EncodeText`
does not add framing. Sample timing, audio output, and streaming queues remain
unimplemented.

Frame codes separately into logical mark/space symbols:

```go
symbols, err := rtty.FrameCodes(encoded.Codes, rtty.StopBitsOnePointFive)
if err != nil {
    return err
}
fmt.Println(len(symbols)) // 42: seven symbols per code
```

Each frame contains a space start, five LSB-first data bits, and a mark stop.
Start and data symbols last two half-bits; stop lasts three or four for 1.5 or
2 stop bits. Adjacent equal states remain separate. Framing adds no shifts,
lead-in, or tail, and is independent of audio polarity and sample timing.
Invalid stop selections return `ErrInvalidOptions` with `*OptionsError` before
codes are inspected. The first value above 31 returns `ErrInvalidInput` with
`*InvalidCodeError`, exposing `CodeIndex` and `Code`. Errors and empty valid
input return nil symbols. Successful output belongs to the caller.

## Planned capabilities

- Decode a selected RTTY signal from continuous mono PCM chunks into character
  and control observations, text, and reception diagnostics.
- Generate AFSK audio through caller-supplied buffers, supporting both complete
  messages and interactive text with a bounded queue.
- Preserve filter, timing, and shift state across receive chunks, and preserve
  symbol timing and oscillator phase across transmit buffers.
- Provide checked configuration, typed errors, explicit resets for audio gaps,
  and independent decoder/transmitter instances.

Applications will own audio capture and playback, sample-rate conversion when
needed, CAT/PTT, radio control, presentation, and QSO logging. The library will
process audio synchronously without opening devices or scheduling transmission
against the wall clock.

## Initial operating profile

The proposed first release focuses on the common 45-baud, 170-Hz amateur RTTY
profile:

| Property | Planned support |
| --- | --- |
| Audio | Mono signed 16-bit PCM at 12 kHz or 48 kHz |
| Baud rate | `500/11` baud, approximately 45.4545; explicit 45.45 compatibility setting |
| Shift | 170 Hz |
| Framing | One start bit, five data bits sent least significant bit first, no parity, 1.5 or 2 stop bits |
| Alphabet | Explicit US teleprinter letters/figures table |
| Tuning | Caller-selected audio center frequency with fixed receive tuning |
| Polarity | Explicit mark-high or mark-low audio assignment |
| Receive channels | One selected signal per decoder instance |

Later stages propose 50 and 75 baud, wider shifts, an international ITA2
profile, bounded automatic frequency control, and stronger reception under
fading and interference. Each extension requires independent validation before
it becomes advertised support.

Ordinary five-bit RTTY has no CRC or forward error correction. Received
characters and confidence metrics will be observations, not guarantees that
the transmitted text was recovered correctly. Raw codes and shift-state
provenance will remain available alongside rendered text.

## Implementation approach

The project follows the development approach used by
[go-ft8](https://github.com/ColonelBlimp/go-ft8): Go protocol code, checked APIs,
explicit diagnostics, independent reference comparisons, and profiling before
optimization. RTTY will use continuous character streams rather than timed
FT8 receive slots. There is no runtime dependency on `go-ft8`.

The baseline will use Go DSP and support `CGO_ENABLED=0`. A small CGO backend
may be added if reception evaluation or profiling justifies it. Native modem
reuse remains an option if the Go receiver cannot meet the design's reception
requirements.

Validation will combine independently reviewed protocol vectors, bidirectional
interoperability with fldigi and minimodem, labelled recordings, and generated
channel impairments. Receive chunk boundaries and transmit buffer sizes must
not change the resulting protocol stream. Self-round-trips alone will not be
treated as sufficient evidence of correctness.

## Development

The module path is `github.com/ColonelBlimp/go-rtty`, and `go.mod` currently
targets Go 1.26. The public package is
`github.com/ColonelBlimp/go-rtty/rtty`; its implemented subset is described above.

Development will proceed through these stages:

1. Establish pinned reference tools, independent fixtures, and development
   tooling.
2. Implement the alphabet, text encoder, framing, and configuration contracts.
3. Evaluate a receiver prototype and implement the streaming receive API.
4. Implement streaming AFSK generation and verify it with independent receivers.
5. Meet the reception, interoperability, resource, and performance gates before
   preparing a release.

The implemented Task commands follow the `go-ft8` naming conventions:

```sh
go test ./...
CGO_ENABLED=0 go test ./...
task check                 # version, Go/Python tests, vet, race
task corpus:check          # offline Python tooling checks; optional resampler qualification
task corpus:check-captures # admission and actual Go capture/scoring on known-TX test audio
task receiver:baseline     # internal receiver results on independent clean WAVs
task receiver:impairments  # reproducible development errors and chunk invariance
task receiver:impairments:legacy # reproduce the original receiver comparison
task receiver:selective    # unequal-tone candidates and existing-case regressions
task receiver:noise        # guarded envelope estimates and half-bit evidence
task receiver:paths        # fade traces and causal two-path comparisons
task test:race-paths       # targeted race checks for the new channel work
task bench:receiver        # clean receiver benchmark with 20 ms chunks
task bench:thresholds      # test-only threshold candidates with replay overhead
task bench:noise           # guarded-envelope prototype costs
task bench:paths           # clean costs of the channel-study candidates
task --list                # implemented tasks only
task version:get
```

Use `GOCACHE=/tmp/go-build` if the default build cache is not writable. The
ordinary suites need only Go 1.26 and the checked-in fixtures. Race checks
also need a supported CGO toolchain. External reference checks additionally
need the pinned executables and Python; see [REFERENCE.md](docs/REFERENCE.md).
No task loads environment files or downloads tools implicitly.

`version.txt` is an unreleased development identifier. Tagging, remote
publication, receiver corpus gates, and broader benchmark tasks remain future work.
Other task names in the design remain proposals until implemented.

Read [AGENTS.md](AGENTS.md) for repository instructions and the
[delivery plan](docs/RTTY_DESIGN.md#delivery-plan) for stage-specific exit gates.

## License

`go-rtty` is licensed under the GNU General Public License, version 3 only
(`GPL-3.0-only`), matching `go-ft8`. See [LICENSE](LICENSE) for the complete
license text.

Original contributions are copyright (C) 2026 Marc L. Veary (7Q5MLV).
Any reused or vendored material retains its upstream copyright and license
notices, with its provenance recorded when introduced.
