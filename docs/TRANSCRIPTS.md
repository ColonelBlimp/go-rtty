<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# External transcript review and development scoring

`scripts/transcripts.py` prepares review packets, checks transcript admission,
creates matched decoder inputs, and scores development comparisons. It does not
supply trusted HF text. The GRITTY and W6IWI manifests remain **unlabelled**, with
no scored ranges. No HF transcript has been admitted by this work.

All recordings, editable labels, review audio, admission records, captures and
scores remain in an external cache. Commands work offline, never fetch or
resample implicitly, reject paths that escape the cache (including symlinks),
and never replace an existing output. Use a new output name for a new revision.
Normal Go tests skip external capture; the Python tests need no external tools.

## Review a recording

The source fixture must already be cached and checksum-verified. For example:

```sh
export RTTY_CORPUS=/tmp/go-rtty-review-corpus
python3 -B scripts/corpus.py fetch --corpus gritty \
  --fixture sx150itu_2015-04-13-135142
python3 -B scripts/transcripts.py draft --corpus gritty \
  --fixture sx150itu_2015-04-13-135142 \
  --start-sample 0 --end-sample 60000 --output review-draft.json
python3 -B scripts/transcripts.py packet --labels review-draft.json \
  --output review-packet-sx150itu
```

The first ten seconds are a **draft review window**, not an asserted character
boundary. `packet` extracts original-rate PCM without changing its samples.
The packet contains WAVs, their hashes/bounds in `index.json`, editable
`labels.json`, and `REVIEW.txt`. Window bounds are integers in the original
recording's sample clock, half-open `[start_sample, end_sample)`. IDs must be
unique and windows ordered and nonoverlapping. Split windows longer than 120
seconds; a label file supports at most 128 windows.
If you change boundaries, generate a new packet and review its actual WAVs
before approving the revised labels; the old packet's audio keeps its old bounds.

Review the audio and edit the packet's `labels.json`:

- Establish truth from known transmitter text or an independent review. State
  the evidence kind, description and references. Decoder agreement, a source
  page's generic pattern description, or a plausible callsign is insufficient.
- Verify the baud, tones, polarity implied by those tones, stop interval and
  alphabet; record the evidence. Draft settings are provisional. This adapter
  currently supports the qualified `909/20` baud, 170 Hz, US alphabet profile,
  integer-Hz tones and 1.5/2 stops at 12/48 kHz. Its guard limits are shared
  between both Go candidates. It does not silently substitute another profile.
- Choose boundaries with real acquisition lead and complete final frames/tail.
  Each window starts a fresh receiver in letters state. Explain why that state
  and those boundaries are justified in `boundary_evidence`. No preceding
  decoder state is carried in, and no padding or synthetic flush is added.
- Mark scoreable windows `reviewed`. Supply `expected.off` and `expected.on`
  separately, even if equal. They contain the declared printable ASCII view,
  preserving exact spaces, case and punctuation. `null` means missing truth;
  `""` means reviewed silence/no printable text and counts all false output.
- Exclude uncertain intervals before capture/scoring. Use a separate window
  with `status: "excluded"`, `expected: null`, no expected codes, and a reason.
  It is not decoded or scored. Samples outside reviewed windows are unscored.
  A reviewed window cannot contain unknown characters to be dropped later.
- If independently known, provide `expected_codes` as five-bit integers in
  wire order, including shifts and controls. Otherwise leave it `null`; raw
  code scoring is then explicitly unavailable. Never fill it from Go output.

Review status is an explicit human attestation. Set `review.status` to
`approved`, name the reviewer, supply a UTC ISO timestamp and notes, and affirm
`independent_of_decoder_outputs: true` only when justified. After all changes
outside the `review` object, obtain the subject hash and put it in
`review.subject_sha256`:

```sh
python3 -B scripts/transcripts.py check --labels review-packet-sx150itu/labels.json
python3 -B scripts/transcripts.py subject --labels review-packet-sx150itu/labels.json
python3 -B scripts/transcripts.py check --require-reviewed \
  --labels review-packet-sx150itu/labels.json
python3 -B scripts/transcripts.py admit \
  --labels review-packet-sx150itu/labels.json --output review/admitted-v1.json
```

Plain `check` validates structure and pins; it does **not** require approval.
`check --require-reviewed` and `admit` reject missing truth, draft windows,
provisional settings, missing boundary evidence, or a changed review subject.
Admission embeds the complete labels and their canonical hash in a new file.
The subject excludes only `review`; admission's labels hash includes it.
These hashes detect subsequent changes, not reviewer identity or factual truth.
An attestation is not a cryptographic signature. Admission also verifies the
pinned original audio. It cannot convert existing development groups to
held-out evidence, and does not edit the checked-in corpus manifest.

## Capture the same admitted windows

Explicitly prepare the pinned derivatives first, using the libsamplerate
version from [REFERENCE.md](REFERENCE.md). Missing tools/audio fail; there is
no fallback resampler. Preparation below creates both full-file rates:

```sh
python3 -B scripts/corpus.py prepare --corpus gritty \
  --fixture sx150itu_2015-04-13-135142
python3 -B scripts/transcripts.py prepare --admission review/admitted-v1.json \
  --sample-rate 12000 --output review/plan-v1.json
python3 -B scripts/transcripts.py capture-go --plan review/plan-v1.json \
  --output review/go-v1.json
python3 -B scripts/transcripts.py capture-fldigi --plan review/plan-v1.json \
  --output review/fldigi-v1.json
python3 -B scripts/transcripts.py score --plan review/plan-v1.json \
  --capture review/go-v1.json --capture review/fldigi-v1.json \
  --output review/score-v1.json
```

`prepare` takes exact subranges of the pinned whole-file derivative. Boundary
mapping is `floor(original_boundary * output_rate / original_rate)` for each
endpoint, including the noninteger 8-to-12 kHz mapping. The plan binds the
admission file, labels, configuration, lifecycle, USOS modes, clip bytes and
original boundaries. It omits expected text/codes. Capture preflight and scoring
reconstruct the clips from the pinned derivative, rejecting changed boundaries
or audio even if someone also changes the clip hash in the plan.

The Go driver is a test-only entry point, `TestReviewedCorpusCapture`. It reads
only the plan and WAVs, runs `go-ratio` and the frozen `go-guarded-short` candidate
with both USOS policies, and retains full codes, text and sample observations.
Each run must match a whole-buffer replay with a 257-sample partition. WAV and
JSON reads are bounded before allocation. Reports record source/parameter
hashes and the Go version. The receiver implementation and candidate parameters
are unchanged.
JSON inputs are limited to 8 MiB; use smaller batches if a long capture report
exceeds that limit.

Fldigi capture uses the pinned application and existing isolated Xvfb adapter,
fresh configuration/state per clip/mode, fixed tuning, AFC and squelch off,
TX USOS off, and both RX USOS modes. It retains the complete widget capture,
configuration XML/hash, observed receiver settings and executable/tool/adapter
hashes. A failed replay retains a report marked `failed`; it cannot be scored.
The application provides no raw-code capture here. GUI CR/LF rendering also
prevents treating widget output as exact raw text.

## Interpret scores

The default comparison requires all three decoders, both modes and every
reviewed window. Missing, extra or duplicate results, failed reports, and
plan/configuration/audio mismatches fail without publishing a score. A fldigi
report must also identify the pinned executable and current tool lock. An
explicit `--require-decoders go-ratio,go-guarded-short` permits Go-only work;
its result says `partial-comparison` and lists fldigi as unperformed.

The `printable-ascii` projection removes only C0 controls and DEL from complete
captured output. It preserves spaces, case, punctuation and unexpected Unicode
(the latter still counts as an error). It performs no substring selection,
whitespace folding, case conversion, or per-decoder trim. Reference expectations
must be printable ASCII. Original captures retain everything before projection.
This declared auxiliary view does **not** satisfy the design's exact-text
release gate; every score records `exact_text_scored: false` and
`release_acceptance: false`.

Minimum-edit counts report substitutions, deletions and insertions per window,
decoder and USOS policy, with text totals kept separate by policy. CER is total
edits divided by reference characters, and may exceed one. Empty references
have `cer: null` and retain all insertions; inspect those window rows for false
output rather than interpreting a null rate as success. If expected raw codes
exist, Go rows also report code edit counts/rate; unavailable raw-code truth
is `null`. The deterministic edit tie preference is substitution, deletion,
then insertion. Very large edit matrices fail and require smaller reviewed
windows (maximum four million cells).

Capture reports are provenance records, not authenticated messages. Preserve
them and the bound admission/plan together. Scores identify capture-file hashes
and the scoring driver. Historical source hashes remain attributable even if
a later checkout changes. No tool can prove that handwritten expected labels
or capture records are truthful.

## Validation record (6 November 2026)

The offline suite exercises draft rejection, independent-review requirements,
review-subject changes, boundaries/exclusions/group identity, exact derivative
subranges, fractional source mapping, duplicate/missing/failed captures, both
USOS policies, spaces/case/extra output, empty references, bounded edit matrices,
JSON duplicates, path escapes and exclusive publication. A mocked fldigi
adapter checks matched clips, mode/configuration plumbing and retained failures;
it does not qualify a real fldigi replay.

`task corpus:check-captures` additionally runs actual Go capture/admission/scoring
on the checked-in independent USOS probe, using a temporary **synthetic test
manifest**. The existing 12 kHz known-transmitter WAV is both source and
pinned derivative in this test; no resampler is tested. Both candidates recover
the exact six codes and both printable expectations with zero edits. The Go
capture configuration/replay tests also cover all nine independent WAVs across
both polarities, stop selections and rates, with both USOS settings and
whole/chunked observation equality.

Ordinary and pure-Go suites, vet, Python tooling with explicit Go integration,
and targeted race checks are recorded in [REFERENCE.md](REFERENCE.md).
The pinned resampler and actual fldigi/Xvfb replay are unavailable in this
session and remain unperformed for this workflow. One original GRITTY file
was downloaded and verified, and an original-rate ten-second draft packet was
created externally. It contains no approved labels. There are no new labelled
HF results, no held-out groups, and no public API changes.
