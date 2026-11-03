# go-rtty

`go-rtty` is a project to build a standalone Go library for RTTY reception,
text and protocol encoding, and audio frequency-shift keying (AFSK) generation.
It is intended for applications that need a streaming modem while retaining
control of their audio devices and station operation.

## Status

The project has its initial reference and development foundation: pinned
external tools, independent protocol/audio fixtures, artifact validation tests,
Task commands, and a Linux CI workflow. The modem, public `rtty` API, and
encode/decode command-line tools are not implemented yet.

The ordinary tests validate the retained evidence; they do not test a go-rtty
modem. See the [reference recipe and validation record](docs/REFERENCE.md) and
[corpus inventory](docs/CORPUS.md) for what has actually been exercised and
what remains unperformed.

An explicit [external corpus workflow](docs/REFERENCE.md#external-gritty-development-workflow)
now fetches five checksum-pinned GRITTY recordings, prepares fixed 12/48 kHz
derivatives, and captures unscored fldigi output. Audio and captures remain in a
caller-selected external cache. Trusted transcripts and held-out reception
validation are still outstanding.

The [design document](docs/RTTY_DESIGN.md) specifies the proposed architecture,
streaming contracts, receive and transmit processing, validation requirements,
performance targets, and delivery stages. API examples and acceptance targets
in that document describe planned behavior.

## Planned capabilities

- Decode a selected RTTY signal from continuous mono PCM chunks into character
  and control observations, text, and reception diagnostics.
- Encode text into five-bit codes, letters/figures shifts, and timed mark/space
  symbols.
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
targets Go 1.26. The planned public package is
`github.com/ColonelBlimp/go-rtty/rtty`; it is not available in this checkout yet.

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
task --list                # implemented tasks only
task version:get
```

Use `GOCACHE=/tmp/go-build` if the default build cache is not writable. The
ordinary suites need only Go 1.26 and the checked-in fixtures. Race checks
also need a supported CGO toolchain. External reference checks additionally
need the pinned executables and Python; see [REFERENCE.md](docs/REFERENCE.md).
No task loads environment files or downloads tools implicitly.

`version.txt` is an unreleased development identifier. Tagging, remote
publication, receiver corpus gates, and benchmark tasks remain future work.
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
