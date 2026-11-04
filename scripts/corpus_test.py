# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Offline corpus-tool failure checks; optional pinned-resampler qualification."""

import io
import json
import os
from pathlib import Path
import struct
import subprocess
import tempfile
import unittest
from unittest.mock import patch
import wave

import corpus
import reference


class CorpusTests(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory(prefix="go-rtty-corpus-test-")
        self.addCleanup(self.temporary.cleanup)
        self.cache = Path(self.temporary.name).resolve()
        self.data = corpus.wav_bytes(struct.pack("<hhhh", 0, 123, -456, 0), 6000)
        self.original = {"path": "original/probe.wav", "url": "https://example.invalid/probe.wav",
                         "bytes": len(self.data), "sha256": reference.digest(self.data),
                         "sample_rate": 6000, "sample_count": 4}
        self.fixture = {"id": "probe", "original": self.original, "derived": []}

    def test_rejects_repository_and_symlink_escapes(self):
        with self.assertRaises(ValueError):
            corpus.external_cache(reference.ROOT / "testdata")
        (self.cache / "original").symlink_to(reference.DATA, target_is_directory=True)
        for name in ["original/probe.wav", "../escape.wav", str(reference.DATA / "probe.wav")]:
            with self.subTest(name=name), self.assertRaises(ValueError):
                corpus.cache_path(self.cache, name)

    def test_publish_does_not_replace_existing_file(self):
        path = self.cache / "original.wav"
        path.write_bytes(b"keep me")
        with self.assertRaises(FileExistsError):
            corpus.publish(path, b"replacement")
        self.assertEqual(path.read_bytes(), b"keep me")
        self.assertEqual(list(self.cache.iterdir()), [path])

    def test_corrupt_download_is_not_published(self):
        with patch("urllib.request.urlopen", return_value=io.BytesIO(self.data[:-1])):
            with self.assertRaisesRegex(ValueError, "download checksum/size mismatch"):
                corpus.fetch(self.cache, [self.fixture])
        self.assertEqual(list(self.cache.iterdir()), [])

    def test_existing_cache_is_checked_without_network(self):
        path = self.cache / self.original["path"]
        path.parent.mkdir()
        path.write_bytes(self.data)
        with patch("urllib.request.urlopen", side_effect=AssertionError("network access")):
            corpus.fetch(self.cache, [self.fixture])
            corpus.verify(self.cache, [self.fixture])
            path.write_bytes(self.data[:-1])
            with self.assertRaisesRegex(ValueError, "checksum/size mismatch"):
                corpus.fetch(self.cache, [self.fixture])
        self.assertEqual(path.read_bytes(), self.data[:-1])

    def test_missing_external_audio_fails_offline_verification(self):
        with patch("urllib.request.urlopen", side_effect=AssertionError("network access")):
            with self.assertRaises(FileNotFoundError):
                corpus.verify(self.cache, [self.fixture])

    def test_rejects_truncated_payload_and_wrong_sample_rate(self):
        truncated = bytearray(self.data[:-2])
        struct.pack_into("<I", truncated, 4, len(truncated)-8)
        with self.assertRaisesRegex(ValueError, "truncated PCM"):
            corpus.pcm(truncated, self.original)
        with self.assertRaisesRegex(ValueError, "parameters disagree"):
            corpus.pcm(self.data, {**self.original, "sample_rate": 12000})

    def test_recipe_is_locked(self):
        for name, rate in [("gritty", 6000), ("w6iwi", 8000)]:
            with self.subTest(corpus=name):
                locked = corpus.manifest(name)
                self.assertEqual(locked["conversion"]["settings"], {**corpus.CONVERSION, "input_rate": rate})
                self.assertTrue(all(f["label_status"] == "unlabelled" and not f["scored_ranges"]
                                    for f in locked["fixtures"]))

    def test_websdr_header_exception_does_not_allow_missing_pcm(self):
        data = bytearray(self.data)
        struct.pack_into("<I", data, 4, len(data))
        with self.assertRaisesRegex(ValueError, "invalid RIFF"):
            corpus.pcm(data, self.original)
        annotated = {**self.original, "riff_size_overstatement_bytes": 8}
        self.assertEqual(corpus.pcm(data, annotated), self.data[44:])
        # A truncated file with the same header exception must still fail.
        truncated = data[:-2]
        struct.pack_into("<I", truncated, 4, len(truncated))
        with self.assertRaisesRegex(ValueError, "truncated PCM"):
            corpus.pcm(truncated, annotated)
        with self.assertRaisesRegex(ValueError, "invalid RIFF"):
            corpus.pcm(data, {**annotated, "riff_size_overstatement_bytes": 16})

    def test_failed_minimodem_capture_is_retained_and_never_replaced(self):
        path = self.cache / self.original["path"]
        path.parent.mkdir()
        path.write_bytes(self.data)
        fixture = {**self.fixture, "derived": [self.original],
                   "reference_settings": {"mark_hz": 2125, "space_hz": 2295, "stop_half_bits": 3}}
        failure = subprocess.CalledProcessError(1, ["minimodem"], stderr=b"failure")
        with patch.object(reference, "checked_tool", return_value=__file__), \
             patch.object(reference, "run", side_effect=failure):
            with self.assertRaises(subprocess.CalledProcessError):
                corpus.minimodem_capture(self.cache, [fixture], 6000, "reports/failure.json")
            report_path = self.cache / "reports/failure.json"
            retained = report_path.read_bytes()
            report = json.loads(retained)
            self.assertEqual(report["status"], "failed")
            self.assertEqual(report["results"], [])
            with self.assertRaisesRegex(ValueError, "already exists"):
                corpus.minimodem_capture(self.cache, [fixture], 6000, "reports/failure.json")
            self.assertEqual(report_path.read_bytes(), retained)


@unittest.skipUnless(os.environ.get("RTTY_SAMPLERATE"), "pinned resampler unavailable; qualification unperformed")
class ConversionTests(unittest.TestCase):
    def test_impulse_alignment_duration_and_dc_gain(self):
        converter = corpus.Resampler()
        for input_rate, rate in [(6000, 12000), (6000, 48000), (8000, 12000), (8000, 48000)]:
            # Odd frame count exercises floor rounding at the 8 -> 12 kHz ratio.
            size = 2049
            count = size * rate // input_rate
            impulse = [0] * size
            impulse[512] = 16000
            for kind, samples in [("impulse", impulse), ("dc", [16000] * size)]:
                with self.subTest(input_rate=input_rate, rate=rate, signal=kind):
                    data = converter.convert(struct.pack(f"<{size}h", *samples), input_rate, rate)
                    with wave.open(io.BytesIO(data), "rb") as wav:
                        self.assertEqual(wav.getnframes(), count)
                        self.assertEqual(wav.getframerate(), rate)
                        output = struct.unpack(f"<{count}h", wav.readframes(wav.getnframes()))
                    if kind == "impulse":
                        self.assertEqual(max(range(len(output)), key=lambda i: abs(output[i])), 512*rate//input_rate)
                    else:
                        self.assertTrue(all(abs(value-8000) <= 1 for value in output[512*rate//input_rate:1536*rate//input_rate]))


if __name__ == "__main__":
    unittest.main()
