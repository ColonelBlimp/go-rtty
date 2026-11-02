# Agent Instructions

## Project Scope

`go-rtty` is a standalone Go library for streaming RTTY reception, protocol
encoding, and AFSK generation. The module is
`github.com/ColonelBlimp/go-rtty`; the planned public package is `rtty`.

Read [docs/RTTY_DESIGN.md](docs/RTTY_DESIGN.md) before changing the architecture
or public API. It defines the proposed contracts and staged delivery plan;
it does not describe an already implemented modem. Keep documentation aligned
with actual implementation and validation results.

Audio devices, CAT/PTT, radio control, UI, and QSO logging belong to callers.
Keep them outside the library. Work in this repository does not authorize
changes to Station Manager or other repositories.

## Secret Handling

Do not read, print, cat, grep, parse, summarize, diff, or otherwise inspect
secret-bearing local files, including:

- `.env`
- `.env.*`
- `*.secret`
- `*.pem`
- `id_*`
- `*_token*`
- Files or paths clearly containing credentials, private keys, access tokens,
  or personal secrets

If a task needs an environment variable such as `GITHUB_TOKEN`, use it only
via the process environment or Taskfile execution. Do not open the file that
defines the variable.

If secret contents appear in command output accidentally, do not repeat them
in the response.

## Implementation and Validation

Preserve a working pure-Go build with `CGO_ENABLED=0`. Add CGO dependencies
only when receiver evaluation or profiling justifies them, and retain
explicit native resource ownership and upstream provenance.

Receive state must persist across arbitrary PCM chunks. Transmit timing and
oscillator phase must persist across output buffers. Keep state independent
between instances and make resets and discontinuities explicit.

Use independent protocol vectors and transmitter/receiver fixtures in addition
to self-round-trips. Do not describe framed RTTY characters as CRC-verified
messages or confidence scores as probabilities of correctness.

Run `gofmt` on changed Go files and the checks appropriate to the change.
Changes to DSP, framing, or streaming state require the relevant corpus and
chunk-invariance checks. Report unavailable reference tools or external
fixtures as unperformed checks, not passing results.

## Project Workflow

The module currently targets Go 1.26. Follow the Task-based workflow from
`go-ft8` as development tooling is introduced. Inspect the actual Taskfiles
before invoking tasks; the commands listed in the design document are planned
until implemented.

Once Go packages exist, use `go test ./...` and `CGO_ENABLED=0 go test ./...`
for the ordinary and pure-Go suites. Race checks require a supported CGO
toolchain; use the planned `purego` build tag to select Go DSP while retaining
CGO for race instrumentation when backend selection is implemented.

Use `GOCACHE=/tmp/go-build` for Go commands when the default build cache is not
writable. Preserve unrelated working-tree changes. Create commits, version
tags, and remote publications only within the user's requested scope.

## Licensing and Attribution

This project is licensed under the GNU General Public License, version 3 only
(`GPL-3.0-only`). This applies to original project code, tests, and
documentation unless a file states otherwise. The complete license text is
in [LICENSE](LICENSE).

Original contributions use the following notices where practical:

```text
SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
SPDX-License-Identifier: GPL-3.0-only
```

Retain upstream copyright and license notices for copied, translated,
adapted, or vendored material. Record its source, immutable revision,
modifications, and applicable license in `docs/PROVENANCE.md` when such
material is introduced. Do not claim ownership of upstream work or describe
source-derived code as a clean-room implementation.
