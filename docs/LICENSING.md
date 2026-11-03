<!-- SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV) -->
<!-- SPDX-License-Identifier: GPL-3.0-only -->

# Licensing

Original go-rtty code, tests, documentation, and generated fixture audio are
licensed under **GPL-3.0-only**. The complete license is in [LICENSE](../LICENSE).
The copyright holder for original contributions is Marc L. Veary (7Q5MLV), 2026.

Files adapted from upstream material retain their original notices and license
terms. In particular, the alphabet transcription retains fldigi's
GPL-3.0-or-later notice in [testdata/protocol/NOTICE](../testdata/protocol/NOTICE).
Do not replace that upstream notice with the project's original-work notice.

Reference executables and their runtime libraries are external development
tools. They are downloaded only by an explicit command, are not vendored, and
are not dependencies of the Go library or ordinary tests. Their own licenses
continue to apply. Running an external tool does not assign its source-code
copyright to go-rtty or make our authored text its output's source code.

[PROVENANCE.md](PROVENANCE.md) records the source and treatment of retained
material. New recordings require known origin, transcript provenance, and
redistribution permission before inclusion. A publicly downloadable recording
does not by itself establish permission to redistribute it.
