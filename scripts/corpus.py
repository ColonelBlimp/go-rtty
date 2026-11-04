#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Explicit acquisition, conversion and unscored replay of external RTTY audio.

Only fetch accesses the network. No command changes the checked-in manifest.
Audio, logs and captures must live outside the repository.
"""

import argparse
import array
import ctypes
from datetime import datetime, timezone
import importlib.util
import io
import json
import os
from pathlib import Path
import struct
import subprocess
import sys
import tempfile
import urllib.request
import wave

sys.dont_write_bytecode = True
import reference

MANIFESTS = {name: reference.DATA / f"manifests/{name}.json" for name in ("gritty", "w6iwi")}
CONVERSION = {"input_rate": 6000, "output_rates": [12000, 48000], "input_gain": 0.5,
              "converter_type": 0, "end_of_input": 1, "channels": 1,
              "rounding": "src_float_to_short_array"}
REFERENCE_PROFILE = {"baud_numerator": 909, "baud_denominator": 20, "shift_hz": 170,
                     "data_bits": 5, "start_bits": 1, "alphabet": "US", "afc": False,
                     "squelch": False, "rx_usos": [False, True], "tx_usos": False,
                     "bandwidth_hz": 68, "filter_factor": 1.25}


def manifest(name="gritty"):
    data = json.loads(MANIFESTS[name].read_text())
    if data["schema_version"] != 1:
        raise ValueError("unsupported corpus manifest version")
    conversion = {**CONVERSION, "input_rate": 6000 if name == "gritty" else 8000}
    if data["conversion"]["settings"] != conversion:
        raise ValueError("conversion recipe differs from this adapter")
    settings_hash = reference.digest(json.dumps(conversion, sort_keys=True, separators=(",", ":")).encode())
    library_hash = next(t["executable_sha256"] for t in reference.lock()["tools"] if t["name"] == "samplerate")
    if data["conversion"]["configuration_sha256"] != settings_hash or data["conversion"]["library_sha256"] != library_hash:
        raise ValueError("corpus conversion pin differs from the tool/recipe lock")
    if data["reference_profile"] != REFERENCE_PROFILE:
        raise ValueError("reference profile differs from the qualified adapter")
    return data


def external_cache(path):
    path = Path(path).resolve()
    if path.is_relative_to(reference.ROOT):
        raise ValueError("the corpus cache must be outside the repository")
    return path


def cache_path(cache, name):
    path = (cache / name).resolve()
    if not path.is_relative_to(cache) or path.is_relative_to(reference.ROOT):
        raise ValueError(f"cache path escapes external cache: {name}")
    return path


def checked_bytes(path, artifact):
    with path.open("rb") as source:
        data = source.read(artifact["bytes"] + 1)
    if len(data) != artifact["bytes"] or reference.digest(data) != artifact["sha256"]:
        raise ValueError(f"{path}: checksum/size mismatch; existing files are never replaced")
    return data


def publish(path, data):
    """Publish a complete file without overwriting any existing destination."""
    path.parent.mkdir(parents=True, exist_ok=True)
    with tempfile.NamedTemporaryFile(dir=path.parent, delete=False) as temporary:
        temp = Path(temporary.name)
        try:
            temporary.write(data)
            temporary.flush()
            os.fsync(temporary.fileno())
            os.link(temp, path)
        finally:
            temp.unlink()


def pcm(data, artifact):
    # W6IWI's three pinned WebSDR originals overstate RIFF length by eight
    # bytes. Preserve them byte-for-byte; the data chunk must still be complete.
    overstatement = artifact.get("riff_size_overstatement_bytes", 0)
    if overstatement not in (0, 8) or len(data) < 12 or data[:4] != b"RIFF" or data[8:12] != b"WAVE" or struct.unpack_from("<I", data, 4)[0] + 8 != len(data) + overstatement:
        raise ValueError("invalid RIFF size or format")
    with wave.open(io.BytesIO(data), "rb") as source:
        if (source.getnchannels(), source.getsampwidth(), source.getframerate(), source.getnframes(), source.getcomptype()) != (1, 2, artifact["sample_rate"], artifact["sample_count"], "NONE"):
            raise ValueError("WAV parameters disagree with manifest")
        result = source.readframes(source.getnframes())
    if len(result) != artifact["sample_count"] * 2:
        raise ValueError("truncated PCM payload")
    return result


def wav_bytes(samples, rate):
    output = io.BytesIO()
    with wave.open(output, "wb") as target:
        target.setparams((1, 2, rate, 0, "NONE", "not compressed"))
        target.writeframes(samples)
    return output.getvalue()


class SRCData(ctypes.Structure):
    # ABI of libsamplerate 0.2.2 SRC_DATA; API reference in docs/PROVENANCE.md.
    _fields_ = [("data_in", ctypes.POINTER(ctypes.c_float)),
                ("data_out", ctypes.POINTER(ctypes.c_float)),
                ("input_frames", ctypes.c_long), ("output_frames", ctypes.c_long),
                ("input_frames_used", ctypes.c_long), ("output_frames_gen", ctypes.c_long),
                ("end_of_input", ctypes.c_int), ("src_ratio", ctypes.c_double)]


class Resampler:
    def __init__(self):
        self.path = reference.checked_tool("samplerate")
        self.lib = ctypes.CDLL(self.path)
        self.lib.src_simple.argtypes = [ctypes.POINTER(SRCData), ctypes.c_int, ctypes.c_int]
        self.lib.src_simple.restype = ctypes.c_int
        self.lib.src_strerror.argtypes = [ctypes.c_int]
        self.lib.src_strerror.restype = ctypes.c_char_p
        self.lib.src_float_to_short_array.argtypes = [ctypes.POINTER(ctypes.c_float), ctypes.POINTER(ctypes.c_short), ctypes.c_int]
        self.lib.src_float_to_short_array.restype = None

    def convert(self, raw, input_rate, output_rate):
        if output_rate not in CONVERSION["output_rates"] or input_rate not in (6000, 8000):
            raise ValueError("unsupported corpus conversion rate")
        samples = array.array("h", raw)
        if sys.byteorder != "little":
            samples.byteswap()
        n = len(samples)
        count = n * output_rate // input_rate
        # Fixed 0.5 gain before interpolation, no per-file normalization.
        # Avoid expanding long recordings into millions of Python arguments.
        floats = array.array("f", (sample / 32768.0 * CONVERSION["input_gain"] for sample in samples))
        incoming = (ctypes.c_float * n).from_buffer(floats)
        outgoing = (ctypes.c_float * (count + 256))()
        state = SRCData(incoming, outgoing, n, len(outgoing), 0, 0, CONVERSION["end_of_input"], output_rate / input_rate)
        error = self.lib.src_simple(ctypes.byref(state), CONVERSION["converter_type"], CONVERSION["channels"])
        if error:
            raise ValueError(self.lib.src_strerror(error).decode())
        if state.input_frames_used != n or state.output_frames_gen != count:
            raise ValueError(f"resampler consumed/generated {state.input_frames_used}/{state.output_frames_gen}, expected {n}/{count}")
        if any(abs(outgoing[i]) >= 1 for i in range(count)):
            raise ValueError("resampling would introduce full-scale clipping")
        integers = (ctypes.c_short * count)()
        self.lib.src_float_to_short_array(outgoing, integers, count)
        result = array.array("h", bytes(integers))
        if sys.byteorder != "little":
            result.byteswap()
        return wav_bytes(result.tobytes(), output_rate)


def fetch(cache, fixtures):
    for fixture in fixtures:
        original = fixture["original"]
        path = cache_path(cache, original["path"])
        if path.exists():
            data = checked_bytes(path, original)
        else:
            request = urllib.request.Request(original["url"], headers={"User-Agent": "go-rtty-corpus/1.0"})
            with urllib.request.urlopen(request, timeout=30) as response:
                data = response.read(original["bytes"] + 1)
            if len(data) != original["bytes"] or reference.digest(data) != original["sha256"]:
                raise ValueError(f"{fixture['id']}: download checksum/size mismatch")
            pcm(data, original)
            publish(path, data)
        pcm(data, original)
        print(f"VERIFIED original {fixture['id']}", flush=True)


def prepare(cache, fixtures):
    converter = Resampler()
    for fixture in fixtures:
        original = fixture["original"]
        raw = pcm(checked_bytes(cache_path(cache, original["path"]), original), original)
        for derived in fixture["derived"]:
            path = cache_path(cache, derived["path"])
            # Regenerate even if a cached derivative exists: verify the recipe too.
            data = converter.convert(raw, original["sample_rate"], derived["sample_rate"])
            if len(data) != derived["bytes"] or reference.digest(data) != derived["sha256"]:
                raise ValueError(f"{fixture['id']}: conversion differs from pinned derivative")
            pcm(data, derived)
            if path.exists():
                checked_bytes(path, derived)
            else:
                publish(path, data)
            print(f"VERIFIED conversion {fixture['id']} {derived['sample_rate']} Hz", flush=True)


def verify(cache, fixtures):
    for fixture in fixtures:
        for artifact in [fixture["original"], *fixture["derived"]]:
            pcm(checked_bytes(cache_path(cache, artifact["path"]), artifact), artifact)
        print(f"VERIFIED cached audio {fixture['id']}", flush=True)


def minimodem_capture(cache, fixtures, rate, output, *, corpus_name="gritty"):
    """Capture full text and raw codes; neither is a trusted transcript."""
    executable = reference.checked_tool("minimodem")
    output = cache_path(cache, output)
    if output.exists():
        raise ValueError(f"output already exists: {output}")
    report = {"schema_version": 1, "kind": "unscored-minimodem-capture",
              "status": "running", "corpus": corpus_name,
              "started_utc": datetime.now(timezone.utc).isoformat(),
              "manifest_sha256": reference.digest(MANIFESTS[corpus_name].read_bytes()),
              "driver_sha256": reference.digest(Path(__file__).read_bytes()),
              "tools_manifest_sha256": reference.digest((reference.DATA / "manifests/tools.json").read_bytes()),
              "executable_sha256": reference.digest(Path(executable).read_bytes()),
              "sample_rate": rate, "results": []}
    try:
        for fixture in fixtures:
            original = fixture["original"]
            pcm(checked_bytes(cache_path(cache, original["path"]), original), original)
            derived = next(d for d in fixture["derived"] if d["sample_rate"] == rate)
            source = cache_path(cache, derived["path"])
            pcm(checked_bytes(source, derived), derived)
            settings = fixture["reference_settings"]
            args = ["--rx", "45.45", "--baudot", "--startbits", "1", "--stopbits",
                    str(settings["stop_half_bits"] / 2), "--mark", str(settings["mark_hz"]),
                    "--space", str(settings["space_hz"]), "--bandwidth", "10",
                    "--confidence", "1.5", "--limit", "2.3", "--file", str(source)]
            text_result = reference.run(executable, args)
            code_result = reference.run(executable, ["--binary-output", *args])
            words = code_result.stdout.decode("ascii").splitlines()
            if any(len(word) != 5 or set(word) - {"0", "1"} for word in words):
                raise ValueError("invalid minimodem raw-code capture")
            report["results"].append({
                "fixture": fixture["id"], "original_sha256": original["sha256"],
                "wav_sha256": derived["sha256"], "args": args,
                "binary_args": ["--binary-output", *args], "rx_usos": True,
                "actual_text": text_result.stdout.decode("ascii"),
                "actual_codes": [int(word[::-1], 2) for word in words],
                "text_diagnostics": text_result.stderr.decode("ascii"),
                "code_diagnostics": code_result.stderr.decode("ascii"),
                "status": "captured-unscored", "scored_characters": 0})
            print(f"CAPTURED minimodem {fixture['id']}: {len(words)} codes; unscored", flush=True)
        report["status"] = "captured-unscored"
    except BaseException as error:
        report["status"] = "failed"
        report["error"] = str(error)
        raise
    finally:
        publish(output, (json.dumps(report, indent=2) + "\n").encode())
        print(f"Capture report: {output}", flush=True)


def replay(cache, fixtures, rate, output, *, corpus_name="gritty", rx_usos=(0, 1)):
    # Importing the adapter does not launch tools or access the network.
    spec = importlib.util.spec_from_file_location("fldigi_replay", Path(__file__).with_name("fldigi-replay.py"))
    adapter = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(adapter)
    executable = reference.checked_tool("fldigi")
    xvfb = os.environ.get("RTTY_XVFB")
    if not xvfb:
        raise ValueError("set RTTY_XVFB; see docs/REFERENCE.md")
    # Preflight every selected source before starting a potentially long replay.
    inputs = []
    for fixture in fixtures:
        original = fixture["original"]
        pcm(checked_bytes(cache_path(cache, original["path"]), original), original)
        derived = next(d for d in fixture["derived"] if d["sample_rate"] == rate)
        source = cache_path(cache, derived["path"])
        pcm(checked_bytes(source, derived), derived)
        inputs.append((fixture, derived, source))
    output = cache_path(cache, output)
    if output.exists():
        raise ValueError(f"output already exists: {output}")
    output.parent.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix="replay-", dir=output.parent))
    report = {"schema_version": 1, "kind": "unscored-reference-capture", "status": "running",
              "started_utc": datetime.now(timezone.utc).isoformat(),
              "corpus": corpus_name,
              "manifest_sha256": reference.digest(MANIFESTS[corpus_name].read_bytes()),
              "driver_sha256": reference.digest(Path(__file__).read_bytes()),
              "tools_manifest_sha256": reference.digest((reference.DATA / "manifests/tools.json").read_bytes()),
              "adapter_sha256": reference.digest(Path(adapter.__file__).read_bytes()),
              "executable_sha256": reference.digest(Path(executable).read_bytes()),
              "xvfb_sha256": reference.digest(Path(xvfb).read_bytes()),
              "sample_rate": rate, "work_directory": str(work), "results": []}
    try:
        with adapter.display_server(xvfb, work) as display:
            for fixture, derived, source in inputs:
                settings = fixture["reference_settings"]
                replay_fixture = {"id": fixture["id"], **derived, **settings}
                for usos in rx_usos:
                    print(f"REPLAY {fixture['id']} {rate} Hz USOS={usos} (unscored)", flush=True)
                    result = adapter.replay(executable, replay_fixture, usos, display,
                                            work / (fixture["id"] + f"-{usos}"),
                                            source=source, capture_only=True)
                    result["original_sha256"] = fixture["original"]["sha256"]
                    report["results"].append(result)
        report["status"] = "captured-unscored"
    except BaseException as error:
        report["status"] = "failed"
        report["error"] = str(error)
        raise
    finally:
        publish(output, (json.dumps(report, indent=2) + "\n").encode())
        print(f"Capture report: {output}", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=["fetch", "prepare", "verify", "replay", "minimodem"])
    parser.add_argument("--corpus", choices=MANIFESTS, default="gritty")
    parser.add_argument("--cache", type=Path, default=os.environ.get("RTTY_CORPUS"))
    parser.add_argument("--fixture", help="select one recording by manifest ID")
    parser.add_argument("--sample-rate", type=int, choices=[12000, 48000], default=12000,
                        help="replay input rate; prepare always produces both rates")
    parser.add_argument("--output",
                        help="new capture report path relative to the external cache")
    parser.add_argument("--rx-usos", choices=["both", "off", "on"], default="both",
                        help="replay receive USOS policy; each run starts a fresh receiver")
    args = parser.parse_args()
    try:
        if not args.cache:
            raise ValueError("set RTTY_CORPUS or pass --cache")
        cache = external_cache(args.cache)
        fixtures = [f for f in manifest(args.corpus)["fixtures"] if not args.fixture or f["id"] == args.fixture]
        if not fixtures:
            raise ValueError("unknown fixture")
        if args.command != "replay" and args.rx_usos != "both":
            raise ValueError("--rx-usos only applies to fldigi replay; minimodem always enables RX USOS")
        if args.command == "replay":
            policies = {"both": (0, 1), "off": (0,), "on": (1,)}[args.rx_usos]
            suffix = "" if args.rx_usos == "both" else f"-{args.rx_usos}"
            replay(cache, fixtures, args.sample_rate,
                   args.output or f"reports/{args.corpus}-{args.sample_rate}{suffix}.json",
                   corpus_name=args.corpus, rx_usos=policies)
        elif args.command == "minimodem":
            minimodem_capture(cache, fixtures, args.sample_rate,
                              args.output or f"reports/{args.corpus}-minimodem-{args.sample_rate}.json",
                              corpus_name=args.corpus)
        else:
            {"fetch": fetch, "prepare": prepare, "verify": verify}[args.command](cache, fixtures)
    except (ValueError, OSError, RuntimeError, wave.Error, subprocess.SubprocessError) as error:
        parser.exit(1, f"corpus: {error}\n")


if __name__ == "__main__":
    main()
