# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Offline admission and scoring tests using known-TX audio, never HF labels."""
import copy
from contextlib import nullcontext
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import tempfile
from types import SimpleNamespace
import unittest
from unittest.mock import Mock, patch

import corpus
import reference
import transcripts as tr


class TranscriptTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="rtty-transcript-test-")
        self.addCleanup(self.temporary.cleanup)
        self.cache = Path(self.temporary.name)
        known = json.loads((reference.DATA / "manifests/independent.json").read_text())
        self.known = next(f for f in known["fixtures"] if f["id"] == "usos-12000-mark-high-stop3")
        wav = (reference.DATA / self.known["path"]).read_bytes()
        # Synthetic admission manifest only: the existing 12 kHz known-TX WAV
        # serves as both source and derivative. No conversion is under test.
        rate, samples = 12000, (len(wav)-44)//2
        artifact = {"path": "audio/known.wav", "sha256": reference.digest(wav), "bytes": len(wav),
                    "sample_rate": rate, "sample_count": samples}
        corpus.publish(self.cache / artifact["path"], wav)
        manifest = copy.deepcopy(corpus.manifest("gritty"))
        manifest["fixtures"] = [{"id": "known-tx", "partition": "development", "recording_group": "synthetic-known-tx",
                                 "original": artifact, "derived": [artifact],
                                 "reference_settings": {k: self.known[k] for k in ("mark_hz", "space_hz", "stop_half_bits")}}]
        path = self.cache / "synthetic-manifest.json"
        tr.write(path, manifest)
        self.patch = patch.dict(corpus.MANIFESTS, {"gritty": path})
        self.patch.start()
        self.addCleanup(self.patch.stop)
        self.labels = tr.draft("gritty", "known-tx", 0, samples)

    def approved(self):
        labels = copy.deepcopy(self.labels)
        labels["settings"].update(verified=True, evidence="Synthetic test: independently generated minimodem fixture settings")
        labels["evidence"] = {"kind": "known-transmitter-text", "description": "Synthetic workflow test only; fixed independent TX manifest",
                              "references": ["testdata/manifests/independent.json"]}
        window = labels["windows"][0]
        window.update(status="reviewed", boundary_evidence="Synthetic test: complete known transmission, two mark bits lead/tail; initial letters state",
                      expected={m: tr.printable(self.known[f"expected_text_usos_{m}"]) for m in tr.MODES},
                      expected_codes=self.known["expected_codes"])
        labels["review"].update(status="approved", reviewer="synthetic test (not an HF review)",
                                reviewed_utc=datetime.now(timezone.utc).isoformat(), independent_of_decoder_outputs=True,
                                subject_sha256=tr.subject(labels))
        return labels

    def prepared(self):
        labels = self.approved()
        tr.admit(self.cache, labels, self.cache / "admission.json")
        tr.prepare(self.cache, self.cache / "admission.json", 12000, self.cache / "plan.json")
        return tr.plan(self.cache, self.cache / "plan.json")

    def capture(self, decoders=tr.DECODERS):
        p, labels = self.prepared()
        clip = p["clips"][0]
        result = {"schema_version": 1, "kind": "reviewed-capture", "status": "captured-unscored",
                  "plan_sha256": reference.digest((self.cache / "plan.json").read_bytes()),
                  "configuration_sha256": p["configuration_sha256"], "sources_sha256": {"synthetic-test": reference.digest(b"not a real capture")},
                  "executable_sha256": next(t["executable_sha256"] for t in reference.lock()["tools"] if t["name"] == "fldigi"),
                  "tools_manifest_sha256": reference.digest((reference.DATA / "manifests/tools.json").read_bytes()),
                  "results": []}
        for decoder in decoders:
            for mode in tr.MODES:
                result["results"].append({"decoder": decoder, "window": clip["id"], "rx_usos": mode,
                                          "wav_sha256": clip["sha256"], "text": self.known[f"expected_text_usos_{mode}"],
                                          "codes": self.known["expected_codes"]})
        return result

    def run_score(self, capture, decoders=tr.DECODERS, name="score.json"):
        path = self.cache / "capture.json"
        tr.write(path, capture)
        tr.score(self.cache, self.cache / "plan.json", [path], self.cache / name, decoders)
        return tr.read(self.cache / name)

    def test_packet_is_original_audio_and_draft_stays_unadmitted(self):
        tr.packet(self.cache, self.labels, self.cache / "packet")
        self.assertEqual((self.cache / "packet/window-001.wav").read_bytes(), (self.cache / "audio/known.wav").read_bytes())
        self.assertEqual(tr.read(self.cache / "packet/labels.json")["review"]["status"], "draft")
        with self.assertRaisesRegex(ValueError, "unreviewed"):
            tr.admit(self.cache, self.labels, self.cache / "admission.json")
        self.assertFalse((self.cache / "admission.json").exists())

    def test_admission_requires_complete_independent_review(self):
        for mutate in (lambda x: x["settings"].update(verified=False),
                       lambda x: x["evidence"].update(kind="decoder-consensus"),
                       lambda x: x["review"].update(independent_of_decoder_outputs=False),
                       lambda x: x["review"].update(reviewed_utc="2026-11-06T10:00:00"),
                       lambda x: x["windows"][0].update(boundary_evidence=""),
                       lambda x: x["windows"][0]["expected"].update(on=None)):
            labels = self.approved()
            mutate(labels)
            labels["review"]["subject_sha256"] = tr.subject(labels)
            with self.subTest(labels=labels), self.assertRaises((ValueError, TypeError)):
                tr.validate(labels, approved=True)

    def test_subject_binds_text_settings_and_boundaries(self):
        for mutate in (lambda x: x["windows"][0]["expected"].update(on="changed"),
                       lambda x: x["windows"][0].update(end_sample=x["windows"][0]["end_sample"]-1),
                       lambda x: x["settings"].update(stop_half_bits=4)):
            labels = self.approved()
            mutate(labels)
            with self.assertRaisesRegex(ValueError, "subject changed"):
                tr.validate(labels, approved=True)

    def test_bounds_groups_and_exclusions(self):
        for value in (True, -1, 1.5, self.labels["windows"][0]["end_sample"]):
            labels = copy.deepcopy(self.labels)
            labels["windows"][0]["start_sample"] = value
            with self.assertRaises(ValueError):
                tr.validate(labels)
        labels = copy.deepcopy(self.labels)
        labels["partition"] = "held-out"
        with self.assertRaisesRegex(ValueError, "development"):
            tr.validate(labels)
        labels = copy.deepcopy(self.labels)
        labels["windows"].append(copy.deepcopy(labels["windows"][0]))
        labels["windows"][-1]["id"] = "overlap"
        with self.assertRaisesRegex(ValueError, "overlapping"):
            tr.validate(labels)
        labels = self.approved()
        labels["windows"][0].update(status="excluded", expected=None, expected_codes=None, reason="uncertain")
        labels["review"]["subject_sha256"] = tr.subject(labels)
        with self.assertRaisesRegex(ValueError, "no reviewed"):
            tr.validate(labels, approved=True)

    def test_plan_is_blind_and_tampering_fails(self):
        p, _ = self.prepared()
        self.assertNotIn("expected", json.dumps(p))
        clip = p["clips"][0]
        audio = self.cache / clip["path"]
        wav = bytearray(audio.read_bytes())
        wav[-1] ^= 1
        audio.write_bytes(wav)
        clip["sha256"] = reference.digest(wav)
        (self.cache / "plan.json").write_text(json.dumps(p))
        with self.assertRaisesRegex(ValueError, "pinned derivative"):
            tr.plan(self.cache, self.cache / "plan.json")

    def test_score_keeps_usos_separate_and_raw_codes(self):
        report = self.run_score(self.capture())
        self.assertEqual(report["status"], "complete")
        self.assertFalse(report["release_acceptance"])
        self.assertFalse(report["exact_text_scored"])
        self.assertTrue(all(t["edits"] == 0 for t in report["totals"]))
        self.assertEqual({r["expected_printable"] for r in report["results"]}, {"A1 -", "A1 A"})
        self.assertTrue(all(r["code_errors"]["edits"] == 0 for r in report["results"] if r["decoder"] != "fldigi"))

    def test_missing_duplicate_failed_or_mismatched_results_fail(self):
        capture = self.capture()
        for mutate in (lambda x: x["results"].pop(),
                       lambda x: x["results"].append(copy.deepcopy(x["results"][0])),
                       lambda x: x.update(status="failed"),
                       lambda x: x.update(plan_sha256="bad"),
                       lambda x: x.update(executable_sha256="bad"),
                       lambda x: x["results"][0].update(wav_sha256="bad"),
                       lambda x: x["results"][0].update(rx_usos="guessed")):
            bad = copy.deepcopy(capture)
            mutate(bad)
            path = self.cache / "bad.json"
            path.write_text(json.dumps(bad))
            with self.assertRaises(ValueError):
                tr.score(self.cache, self.cache / "plan.json", [path], self.cache / "score.json")
        self.assertFalse((self.cache / "score.json").exists())

    def test_partial_comparison_is_explicit(self):
        capture = self.capture(("go-ratio", "go-guarded-short"))
        report = self.run_score(capture, ("go-ratio", "go-guarded-short"))
        self.assertEqual(report["status"], "partial-comparison")
        self.assertEqual(report["unperformed_decoders"], ["fldigi"])

    def test_fractional_source_boundaries_use_fixed_floor(self):
        path = corpus.MANIFESTS["gritty"]
        manifest = tr.read(path)
        fixture = manifest["fixtures"][0]
        for key, rate, samples in (("original", 8000, 10), ("derived", 12000, 15)):
            audio = corpus.wav_bytes(b"\x00\x00"*samples, rate)
            artifact = {"path": f"audio/silence-{rate}.wav", "sha256": reference.digest(audio),
                        "bytes": len(audio), "sample_rate": rate, "sample_count": samples}
            corpus.publish(self.cache / artifact["path"], audio)
            fixture[key] = [artifact] if key == "derived" else artifact
        path.write_text(json.dumps(manifest))
        self.labels = tr.draft("gritty", "known-tx", 1, 3)
        labels = self.approved()
        labels["windows"][0].update(expected={m: "" for m in tr.MODES}, expected_codes=[])
        labels["review"]["subject_sha256"] = tr.subject(labels)
        tr.admit(self.cache, labels, self.cache / "admission.json")
        tr.prepare(self.cache, self.cache / "admission.json", 12000, self.cache / "plan.json")
        p, _ = tr.plan(self.cache, self.cache / "plan.json")
        self.assertEqual(p["clips"][0]["sample_count"], 3)  # [floor(1.5), floor(4.5))

    def test_fldigi_adapter_gets_same_clips_both_modes_and_keeps_failure(self):
        p, _ = self.prepared()
        executable = self.cache / "synthetic-executable"
        executable.write_bytes(b"not an executable: mocked adapter only")
        replay = Mock(side_effect=lambda f, clip, usos, *args, **kwargs: {"actual_widget_text": "A1 A" if usos else "A1 -"})
        adapter = SimpleNamespace(__file__=str(reference.ROOT / "scripts/fldigi-replay.py"),
                                  display_server=lambda *args: nullcontext(":mock"), replay=replay)
        with patch.object(tr, "fldigi_adapter", return_value=adapter), patch.object(reference, "checked_tool", return_value=str(executable)), patch.dict(os.environ, {"RTTY_XVFB": str(executable)}):
            tr.capture_fldigi(self.cache, self.cache / "plan.json", self.cache / "fldigi.json")
            self.assertEqual(replay.call_count, 2)
            for call, mode in zip(replay.call_args_list, (False, True)):
                self.assertEqual(call.args[1]["sha256"], p["clips"][0]["sha256"])
                self.assertEqual(call.args[1]["mark_hz"], p["configuration"]["mark_hz"])
                self.assertEqual(call.args[2], mode)
                self.assertTrue(call.kwargs["capture_only"])
                self.assertEqual(call.kwargs["source"], self.cache / p["clips"][0]["path"])
            report = tr.read(self.cache / "fldigi.json")
            self.assertEqual(report["status"], "captured-unscored")
            replay.side_effect = RuntimeError("synthetic replay failure")
            with self.assertRaisesRegex(RuntimeError, "synthetic replay failure"):
                tr.capture_fldigi(self.cache, self.cache / "plan.json", self.cache / "failed.json")
            self.assertEqual(tr.read(self.cache / "failed.json")["status"], "failed")

    def test_spaces_case_and_noise_insertions_are_errors(self):
        capture = self.capture()
        capture["results"][0]["text"] = "a1  -EXTRA\r\n"
        report = self.run_score(capture)
        self.assertGreater(report["results"][0]["text_errors"]["cer"], 1)
        self.assertEqual(tr.printable("\x00A\r\n B\x7f\u2603"), "A B\u2603")
        self.assertEqual(tr.edits("AB", "BA")["substitutions"], 2)
        self.assertEqual(tr.edits("", "NOISE")["insertions"], 5)
        self.assertIsNone(tr.edits("", "NOISE")["cer"])
        with self.assertRaisesRegex(ValueError, "too large"):
            tr.edits("A"*4096, "A"*4096)

    def test_json_and_external_paths(self):
        duplicate = self.cache / "duplicate.json"
        duplicate.write_text('{"x": 1, "x": 2}')
        with self.assertRaisesRegex(ValueError, "duplicate JSON"):
            tr.read(duplicate)
        with self.assertRaises(ValueError):
            corpus.cache_path(self.cache, "../escape.json")
        (self.cache / "symlink").symlink_to(reference.ROOT, target_is_directory=True)
        with self.assertRaises(ValueError):
            corpus.cache_path(self.cache, "symlink/escape.json")
        target = self.cache / "never-replace.json"
        tr.write(target, {"first": True})
        with self.assertRaises(FileExistsError):
            tr.write(target, {"second": True})
        self.assertEqual(tr.read(target), {"first": True})

    @unittest.skipUnless(os.environ.get("RTTY_TEST_GO_CAPTURE") == "1", "explicit Go driver integration not requested")
    def test_actual_go_capture_against_known_transmitter(self):
        self.prepared()
        tr.capture_go(self.cache, self.cache / "plan.json", self.cache / "go.json")
        tr.score(self.cache, self.cache / "plan.json", [self.cache / "go.json"], self.cache / "actual-score.json", ("go-ratio", "go-guarded-short"))
        report = tr.read(self.cache / "actual-score.json")
        self.assertTrue(all(t["edits"] == 0 for t in report["totals"]))
        self.assertTrue(all(r["code_errors"]["edits"] == 0 for r in report["results"]))


if __name__ == "__main__":
    unittest.main()
