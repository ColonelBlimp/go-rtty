#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Offline review packets, transcript admission, matched captures and scoring.

All inputs/outputs except pinned manifests and tools stay in an external cache.
Nothing downloads audio, approves a transcript, or changes a corpus manifest.
"""
import argparse
from datetime import datetime, timezone
import importlib.util
import json
import math
import os
from pathlib import Path
import re
import subprocess
import sys
import tempfile

sys.dont_write_bytecode = True
import corpus
import reference

MODES = ("off", "on")
DECODERS = ("go-ratio", "go-guarded-short", "fldigi")
LIMIT = 8 * 1024 * 1024


def require(condition, message):
    if not condition:
        raise ValueError(message)


def canonical(value):
    return json.dumps(value, sort_keys=True, separators=(",", ":"), ensure_ascii=True, allow_nan=False).encode()


def sha(value):
    return reference.digest(canonical(value))


def unique(pairs):
    result = {}
    for key, value in pairs:
        require(key not in result, f"duplicate JSON key: {key}")
        result[key] = value
    return result


def json_bytes(path):
    with path.open("rb") as source:
        data = source.read(LIMIT + 1)
    require(len(data) <= LIMIT, "JSON exceeds 8 MiB limit")
    return data


def read(path):
    return json.loads(json_bytes(path), object_pairs_hook=unique,
                      parse_constant=lambda value: require(False, f"nonfinite JSON: {value}"))


def write(path, value):
    corpus.publish(path, (json.dumps(value, indent=2, ensure_ascii=True, allow_nan=False) + "\n").encode())


def integer(value, low, high):
    return type(value) is int and low <= value <= high


def nonempty(value):
    return isinstance(value, str) and bool(value.strip())


def printable(text):
    # The only declared scoring projection. Keep spaces, case, punctuation and
    # unexpected non-ASCII output. Never collapse whitespace or align a substring.
    return "".join(c for c in text if ord(c) >= 32 and ord(c) != 127)


def subject(labels):
    return sha({k: v for k, v in labels.items() if k != "review"})


def identity(labels):
    name = labels["corpus"]
    require(name in corpus.MANIFESTS, "unknown corpus")
    data = corpus.manifest(name)
    matches = [f for f in data["fixtures"] if f["id"] == labels["fixture"]]
    require(len(matches) == 1, "unknown fixture")
    f = matches[0]
    require(integer(labels["schema_version"], 1, 1) and labels["kind"] == "transcript-review", "unsupported labels")
    require(labels["manifest_sha256"] == reference.digest(corpus.MANIFESTS[name].read_bytes()), "manifest changed")
    require(labels["original_sha256"] == f["original"]["sha256"], "original pin changed")
    require(labels["partition"] == f["partition"] == "development", "only existing development groups supported")
    require(labels["recording_group"] == f["recording_group"], "recording group changed")
    return f


def validate(labels, approved=False):
    f = identity(labels)
    require(labels["scoring_view"] == "printable-ascii", "unsupported scoring view")
    cfg = labels["settings"]
    require(cfg["baud_numerator"] == 909 and cfg["baud_denominator"] == 20 and cfg["alphabet"] == "US", "unqualified reference profile")
    mark, space = cfg["mark_hz"], cfg["space_hz"]
    require(all(type(x) in (int, float) and math.isfinite(x) for x in (mark, space)), "invalid tones")
    require(all(x == int(x) for x in (mark, space)), "fldigi adapter requires integer-Hz tones/carrier")
    require(abs(mark-space) == 170 and min(mark, space) >= 640 and max(mark, space) <= 5360, "tones/guards outside common 12/48 kHz profile")
    require(integer(cfg["stop_half_bits"], 3, 4), "invalid stop interval")
    windows = labels["windows"]
    require(isinstance(windows, list) and 1 <= len(windows) <= 128, "need 1..128 windows")
    seen, end, count = set(), 0, 0
    for w in windows:
        require(isinstance(w["id"], str) and re.fullmatch(r"[a-zA-Z0-9_-]{1,64}", w["id"]) and w["id"] not in seen, "unsafe/duplicate window id")
        seen.add(w["id"])
        a, b = w["start_sample"], w["end_sample"]
        require(integer(a, end, f["original"]["sample_count"]-1) and integer(b, a+1, f["original"]["sample_count"]), "invalid, unsorted or overlapping sample bounds")
        end = b
        require(b-a <= 120 * f["original"]["sample_rate"], "split windows longer than 120 seconds")
        require(w["status"] in ("draft", "reviewed", "excluded"), "invalid window status")
        if w["status"] == "excluded":
            require(nonempty(w["reason"]), "excluded window needs reason")
            require(w["expected"] is None and w.get("expected_codes") is None, "excluded window must have no expected output")
            continue
        expected = w["expected"]
        require(isinstance(expected, dict) and set(expected) == set(MODES), "need both USOS expectations")
        for text in expected.values():
            require(text is None or (isinstance(text, str) and len(text) <= 4096 and all(32 <= ord(c) < 127 for c in text)), "expected printable ASCII only (spaces preserved)")
        codes = w.get("expected_codes")
        require(codes is None or (isinstance(codes, list) and len(codes) <= 4096 and all(integer(c, 0, 31) for c in codes)), "invalid optional raw codes")
        if approved:
            require(w["status"] == "reviewed" and all(isinstance(x, str) for x in expected.values()), "unreviewed window or missing expected text")
            require(nonempty(w["boundary_evidence"]), "need boundary/initial letters-state evidence")
            count += 1
    if approved:
        evidence, review = labels["evidence"], labels["review"]
        require(count > 0, "no reviewed windows")
        require(cfg["verified"] is True and nonempty(cfg["evidence"]), "receiver settings still provisional")
        require(evidence["kind"] in ("known-transmitter-text", "independent-review") and nonempty(evidence["description"]), "missing independent truth evidence")
        require(isinstance(evidence["references"], list) and evidence["references"] and all(isinstance(x, str) and x.strip() for x in evidence["references"]), "missing evidence references")
        require(review["status"] == "approved" and nonempty(review["reviewer"]) and review["independent_of_decoder_outputs"] is True, "missing independent reviewer attestation")
        instant = datetime.fromisoformat(review["reviewed_utc"].replace("Z", "+00:00"))
        require(instant.utcoffset() == timezone.utc.utcoffset(instant), "review time must be UTC")
        require(review["subject_sha256"] == subject(labels), "review subject changed; review again")
    return f


def draft(name, fixture_id, start, end):
    data = corpus.manifest(name)
    f = next((f for f in data["fixtures"] if f["id"] == fixture_id), None)
    require(f is not None, "unknown fixture")
    result = {"schema_version": 1, "kind": "transcript-review", "corpus": name,
              "fixture": f["id"], "manifest_sha256": reference.digest(corpus.MANIFESTS[name].read_bytes()),
              "original_sha256": f["original"]["sha256"], "recording_group": f["recording_group"],
              "partition": f["partition"], "scoring_view": "printable-ascii",
              "settings": {k: data["reference_profile"][k] for k in ("baud_numerator", "baud_denominator", "alphabet")},
              "evidence": {"kind": "pending", "description": "", "references": []},
              "windows": [{"id": "window-001", "start_sample": start, "end_sample": end,
                           "status": "draft", "expected": {m: None for m in MODES},
                           "expected_codes": None, "boundary_evidence": "", "reason": ""}],
              "review": {"status": "draft", "reviewer": "", "reviewed_utc": "",
                         "independent_of_decoder_outputs": False, "subject_sha256": "", "notes": ""}}
    result["settings"].update({k: f["reference_settings"][k] for k in ("mark_hz", "space_hz", "stop_half_bits")})
    result["settings"].update(verified=False, evidence="")
    validate(result)
    return result


def original(cache, labels):
    f = validate(labels)
    a = f["original"]
    return f, corpus.pcm(corpus.checked_bytes(corpus.cache_path(cache, a["path"]), a), a)


def packet(cache, labels, output):
    f, raw = original(cache, labels)
    require(not output.exists(), "packet directory already exists")
    output.mkdir(parents=True)
    index = []
    for w in labels["windows"]:
        audio = corpus.wav_bytes(raw[2*w["start_sample"]:2*w["end_sample"]], f["original"]["sample_rate"])
        path = output / (w["id"] + ".wav")
        corpus.publish(path, audio)
        index.append({"window": w["id"], "wav": path.name, "sha256": reference.digest(audio),
                      "start_sample": w["start_sample"], "end_sample": w["end_sample"]})
    write(output / "labels.json", labels)
    write(output / "index.json", {"status": "draft-review-packet", "original_sha256": labels["original_sha256"], "windows": index})
    corpus.publish(output / "REVIEW.txt", b"Draft only. Listen to the original-rate WAVs; establish truth independently of decoder output.\nEdit labels.json: fixed original sample bounds, settings evidence, expected printable text for USOS off/on, boundary and initial letters-state evidence.\nSplit uncertain spans into excluded windows with reasons; include real acquisition lead and tail. No synthetic padding.\nMark reviewed windows, supply evidence references and an independent reviewer attestation. Run subject after all edits outside review; copy its hash into review.subject_sha256.\nAdmission checks completeness and integrity, not the truth of an attestation. This is development evidence, not release acceptance.\n")


def admit(cache, labels, output):
    validate(labels, approved=True)
    original(cache, labels)
    write(output, {"schema_version": 1, "kind": "admitted-transcript", "labels_sha256": sha(labels),
                   "labels": labels, "admitted_utc": datetime.now(timezone.utc).isoformat(),
                   "driver_sha256": reference.digest(Path(__file__).read_bytes())})


def admission(cache, path):
    record = read(path)
    require(integer(record["schema_version"], 1, 1) and record["kind"] == "admitted-transcript", "not an admission")
    labels = record["labels"]
    require(record["labels_sha256"] == sha(labels), "admission labels changed")
    validate(labels, approved=True)
    return record, labels


def prepare(cache, admitted, rate, output):
    _, labels = admission(cache, admitted)
    f, _ = original(cache, labels)
    a = next(d for d in f["derived"] if d["sample_rate"] == rate)
    raw = corpus.pcm(corpus.checked_bytes(corpus.cache_path(cache, a["path"]), a), a)
    require(not output.exists(), "plan already exists")
    clipdir = output.parent / (output.stem + "-clips")
    require(not clipdir.exists(), "clip directory already exists")
    clipdir.mkdir(parents=True)
    clips = []
    for w in labels["windows"]:
        if w["status"] != "reviewed":
            continue
        # Integer floor for each half-open boundary, fixed before decoding.
        start, end = (w[k] * rate // f["original"]["sample_rate"] for k in ("start_sample", "end_sample"))
        audio = corpus.wav_bytes(raw[2*start:2*end], rate)
        path = clipdir / (w["id"] + ".wav")
        corpus.publish(path, audio)
        clips.append({"id": w["id"], "path": str(path.relative_to(cache)), "sha256": reference.digest(audio),
                      "bytes": len(audio), "sample_rate": rate, "sample_count": end-start,
                      "start_sample": w["start_sample"], "end_sample": w["end_sample"]})
    settings = {k: v for k, v in labels["settings"].items() if k not in ("verified", "evidence")}
    write(output, {"schema_version": 1, "kind": "reviewed-capture-plan", "admission": str(admitted.relative_to(cache)),
                   "admission_sha256": reference.digest(json_bytes(admitted)), "labels_sha256": sha(labels),
                   "configuration": settings, "configuration_sha256": sha(settings), "sample_rate": rate,
                   "initial_state": "fresh-letters", "rx_usos": list(MODES), "clips": clips})


def plan(cache, path):
    p = read(path)
    require(integer(p["schema_version"], 1, 1) and p["kind"] == "reviewed-capture-plan", "not a capture plan")
    apath = corpus.cache_path(cache, p["admission"])
    require(reference.digest(json_bytes(apath)) == p["admission_sha256"], "admission changed")
    _, labels = admission(cache, apath)
    require(p["labels_sha256"] == sha(labels), "wrong labels")
    settings = {k: v for k, v in labels["settings"].items() if k not in ("verified", "evidence")}
    require(p["configuration"] == settings and p["configuration_sha256"] == sha(settings), "wrong settings")
    require(sha(p["configuration"]) == p["configuration_sha256"], "configuration hash mismatch")
    require(p["initial_state"] == "fresh-letters" and p["rx_usos"] == list(MODES), "wrong lifecycle/USOS policy")
    rate = p["sample_rate"]
    require(rate in (12000, 48000), "unsupported sample rate")
    f, _ = original(cache, labels)
    derivative = next(d for d in f["derived"] if d["sample_rate"] == rate)
    raw = corpus.pcm(corpus.checked_bytes(corpus.cache_path(cache, derivative["path"]), derivative), derivative)
    windows = [w for w in labels["windows"] if w["status"] == "reviewed"]
    require(len(p["clips"]) == len(windows), "missing/extra clips")
    for clip, window in zip(p["clips"], windows):
        start, end = (window[k] * rate // f["original"]["sample_rate"] for k in ("start_sample", "end_sample"))
        expected = corpus.wav_bytes(raw[2*start:2*end], rate)
        require(clip["id"] == window["id"] and all(clip[k] == window[k] for k in ("start_sample", "end_sample")), "clip boundary mismatch")
        require(clip["sample_rate"] == rate and clip["sample_count"] == end-start and clip["bytes"] == len(expected) and clip["sha256"] == reference.digest(expected), "clip differs from pinned derivative range")
        corpus.pcm(corpus.checked_bytes(corpus.cache_path(cache, clip["path"]), clip), clip)
    return p, labels


def capture_go(cache, path, output):
    plan(cache, path)
    require(not output.exists(), "capture output already exists")
    env = {**os.environ, "RTTY_CORPUS": str(cache), "RTTY_REVIEW_PLAN": str(path), "RTTY_REVIEW_OUTPUT": str(output)}
    subprocess.run(["go", "test", "./internal/receiver", "-run", "^TestReviewedCorpusCapture$", "-count=1"], cwd=reference.ROOT, env=env, check=True)
    require(output.exists(), "Go capture produced no report")


def fldigi_adapter():
    spec = importlib.util.spec_from_file_location("fldigi_replay", Path(__file__).with_name("fldigi-replay.py"))
    adapter = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(adapter)
    return adapter


def capture_fldigi(cache, path, output):
    p, _ = plan(cache, path)
    require(not output.exists(), "capture output already exists")
    adapter = fldigi_adapter()
    executable = reference.checked_tool("fldigi")
    xvfb = os.environ.get("RTTY_XVFB")
    require(bool(xvfb), "set RTTY_XVFB; see docs/REFERENCE.md")
    output.parent.mkdir(parents=True, exist_ok=True)
    work = Path(tempfile.mkdtemp(prefix="review-replay-", dir=output.parent))
    result = {"schema_version": 1, "kind": "reviewed-capture", "status": "running",
              "plan_sha256": reference.digest(json_bytes(path)), "configuration_sha256": p["configuration_sha256"],
              "sources_sha256": {"scripts/fldigi-replay.py": reference.digest(Path(adapter.__file__).read_bytes()),
                                  "scripts/transcripts.py": reference.digest(Path(__file__).read_bytes())},
              "executable_sha256": reference.digest(Path(executable).read_bytes()),
              "tools_manifest_sha256": reference.digest((reference.DATA / "manifests/tools.json").read_bytes()),
              "xvfb_sha256": reference.digest(Path(xvfb).read_bytes()), "results": []}
    try:
        with adapter.display_server(xvfb, work) as display:
            for clip in p["clips"]:
                f = {**p["configuration"], **clip}
                for mode in MODES:
                    detail = adapter.replay(executable, f, mode == "on", display, work / (clip["id"] + "-" + mode), source=corpus.cache_path(cache, clip["path"]), capture_only=True)
                    result["results"].append({"decoder": "fldigi", "window": clip["id"], "rx_usos": mode,
                                              "wav_sha256": clip["sha256"], "text": detail["actual_widget_text"], "reference_capture": detail})
        result["status"] = "captured-unscored"
    except BaseException as error:
        result["status"], result["error"] = "failed", str(error)
        raise
    finally:
        write(output, result)


def edits(expected, actual):
    """Levenshtein counts, ties: substitution, deletion, insertion. Linear memory."""
    require(len(expected) <= 16384 and len(actual) <= 16384 and len(expected)*len(actual) <= 4_000_000, "edit matrix too large; split review windows")
    previous = [(j, 0, 0, j) for j in range(len(actual)+1)]
    for i, a in enumerate(expected, 1):
        current = [(i, 0, i, 0)]
        for j, b in enumerate(actual, 1):
            cost, s, d, ins = previous[j-1]
            diagonal = (cost+(a != b), s+(a != b), d, ins)
            cost, s, d, ins = previous[j]
            deletion = (cost+1, s, d+1, ins)
            cost, s, d, ins = current[j-1]
            insertion = (cost+1, s, d, ins+1)
            current.append(min((diagonal, deletion, insertion), key=lambda x: x[0]))
        previous = current
    distance, substitutions, deletions, insertions = previous[-1]
    return {"reference_count": len(expected), "actual_count": len(actual), "substitutions": substitutions,
            "deletions": deletions, "insertions": insertions, "edits": distance,
            "cer": distance/len(expected) if expected else None}


def score(cache, path, captures, output, decoders=DECODERS):
    p, labels = plan(cache, path)
    require(decoders and len(set(decoders)) == len(decoders) and set(decoders) <= set(DECODERS), "invalid required decoder set")
    rows, seen = [], set()
    clips = {c["id"]: c for c in p["clips"]}
    windows = {w["id"]: w for w in labels["windows"] if w["status"] == "reviewed"}
    inputs = {}
    for source in captures:
        report = read(source)
        require(integer(report["schema_version"], 1, 1) and report["kind"] == "reviewed-capture" and report["status"] == "captured-unscored", "incomplete/failed capture")
        require(report["plan_sha256"] == reference.digest(json_bytes(path)) and report["configuration_sha256"] == p["configuration_sha256"], "capture plan/configuration mismatch")
        sources = report.get("sources_sha256")
        require(isinstance(sources, dict) and sources and all(isinstance(k, str) and isinstance(v, str) and re.fullmatch(r"[0-9a-f]{64}", v) for k, v in sources.items()), "capture lacks implementation provenance")
        if any(r["decoder"] == "fldigi" for r in report["results"]):
            pinned = next(t for t in reference.lock()["tools"] if t["name"] == "fldigi")
            require(report.get("executable_sha256") == pinned["executable_sha256"] and
                    report.get("tools_manifest_sha256") == reference.digest((reference.DATA / "manifests/tools.json").read_bytes()), "unqualified fldigi executable/tool lock")
        inputs[str(source.relative_to(cache))] = reference.digest(json_bytes(source))
        for row in report["results"]:
            decoder, window, mode = row["decoder"], row["window"], row["rx_usos"]
            require(decoder in decoders and window in clips and mode in MODES, "unexpected decoder/window/USOS")
            key = (decoder, window, mode)
            require(key not in seen, "duplicate capture result")
            seen.add(key)
            require(row["wav_sha256"] == clips[window]["sha256"] and isinstance(row["text"], str), "capture audio/text mismatch")
            actual = printable(row["text"])
            result = {"decoder": decoder, "window": window, "rx_usos": mode,
                      "expected_printable": windows[window]["expected"][mode], "actual_printable": actual,
                      "text_errors": edits(windows[window]["expected"][mode], actual)}
            if decoder != "fldigi":
                codes = row["codes"]
                require(isinstance(codes, list) and all(integer(x, 0, 31) for x in codes), "invalid code capture")
                expected = windows[window].get("expected_codes")
                result["code_errors"] = edits(expected, codes) if expected is not None else None
                if result["code_errors"] is not None:
                    result["code_errors"]["code_error_rate"] = result["code_errors"].pop("cer")
            rows.append(result)
    wanted = {(d, w, m) for d in decoders for w in windows for m in MODES}
    require(seen == wanted, f"missing {len(wanted-seen)} required capture results")
    totals = []
    for decoder in decoders:
        for mode in MODES:
            subset = [r["text_errors"] for r in rows if r["decoder"] == decoder and r["rx_usos"] == mode]
            total = {k: sum(r[k] for r in subset) for k in ("reference_count", "actual_count", "substitutions", "deletions", "insertions", "edits")}
            total["cer"] = total["edits"]/total["reference_count"] if total["reference_count"] else None
            totals.append({"decoder": decoder, "rx_usos": mode, **total})
    write(output, {"schema_version": 1, "kind": "reviewed-development-score", "status": "complete" if set(decoders) == set(DECODERS) else "partial-comparison",
                   "partition": "development", "recording_group": labels["recording_group"], "scoring_view": labels["scoring_view"],
                   "exact_text_scored": False, "release_acceptance": False,
                   "unperformed_decoders": sorted(set(DECODERS)-set(decoders)), "plan_sha256": reference.digest(json_bytes(path)),
                   "labels_sha256": sha(labels), "capture_sha256": inputs, "driver_sha256": reference.digest(Path(__file__).read_bytes()),
                   "results": rows, "totals": totals})


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("command", choices=("draft", "packet", "check", "subject", "admit", "prepare", "capture-go", "capture-fldigi", "score"))
    parser.add_argument("--cache", type=Path, default=os.environ.get("RTTY_CORPUS"))
    parser.add_argument("--corpus", choices=tuple(corpus.MANIFESTS), default="gritty")
    parser.add_argument("--fixture")
    parser.add_argument("--start-sample", type=int, default=0)
    parser.add_argument("--end-sample", type=int)
    parser.add_argument("--labels")
    parser.add_argument("--admission")
    parser.add_argument("--plan")
    parser.add_argument("--capture", action="append", default=[])
    parser.add_argument("--require-decoders", default=",".join(DECODERS))
    parser.add_argument("--sample-rate", type=int, choices=(12000, 48000), default=12000)
    parser.add_argument("--output")
    parser.add_argument("--require-reviewed", action="store_true")
    args = parser.parse_args()
    try:
        require(args.cache is not None, "set --cache or RTTY_CORPUS")
        cache = corpus.external_cache(args.cache)
        def argpath(name):
            value = getattr(args, name)
            require(bool(value), f"--{name} required")
            return corpus.cache_path(cache, value)
        command = args.command
        if command == "draft":
            require(args.fixture and args.end_sample is not None, "--fixture and --end-sample required")
            write(argpath("output"), draft(args.corpus, args.fixture, args.start_sample, args.end_sample))
        elif command in ("check", "subject", "packet", "admit"):
            labels = read(argpath("labels"))
            validate(labels, approved=args.require_reviewed or command == "admit")
            if command == "subject":
                print(subject(labels))
            elif command == "check":
                print("VALID reviewed labels" if args.require_reviewed else "VALID structure (review not required)")
            elif command == "packet":
                packet(cache, labels, argpath("output"))
            else:
                admit(cache, labels, argpath("output"))
        elif command == "prepare":
            prepare(cache, argpath("admission"), args.sample_rate, argpath("output"))
        elif command == "capture-go":
            capture_go(cache, argpath("plan"), argpath("output"))
        elif command == "capture-fldigi":
            capture_fldigi(cache, argpath("plan"), argpath("output"))
        else:
            score(cache, argpath("plan"), [corpus.cache_path(cache, p) for p in args.capture], argpath("output"), tuple(args.require_decoders.split(",")))
    except (ValueError, KeyError, TypeError, AttributeError, OSError, RuntimeError, StopIteration, subprocess.CalledProcessError) as error:
        parser.exit(1, f"ERROR: {error}\n")


if __name__ == "__main__":
    main()
