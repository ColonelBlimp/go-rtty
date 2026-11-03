#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Explicit external reference operations. Ordinary Go tests never run this.

The lock file pins the tested Linux amd64 binaries. Fetching is opt-in and
does not install packages. Verification never updates committed fixtures.
"""

import argparse
import hashlib
import io
import json
import os
from pathlib import Path
import subprocess
import tarfile
import tempfile
import urllib.request

ROOT = Path(__file__).resolve().parents[1]
DATA = ROOT / "testdata"


def digest(data):
    return hashlib.sha256(data).hexdigest()


def lock():
    return json.loads((DATA / "manifests/tools.json").read_text())


def checked_tool(name):
    path = os.environ.get("RTTY_" + name.upper())
    if not path:
        raise ValueError(f"Set RTTY_{name.upper()} to the pinned executable; see docs/REFERENCE.md")
    path = Path(path).resolve(strict=True)
    expected = next(t for t in lock()["tools"] if t["name"] == name)
    if digest(path.read_bytes()) != expected["executable_sha256"]:
        raise ValueError(f"{name}: executable hash differs from tools.json; do not silently replace the oracle")
    return str(path)


def fetch(directory):
    directory = directory.resolve()
    directory.mkdir(parents=True, exist_ok=True)
    artifacts = []
    for tool in lock()["tools"]:
        artifacts.extend([tool["package"], tool["source"]])
    artifacts.extend(lock().get("runtime_packages", []))
    for artifact in artifacts:
        name = artifact["url"].rsplit("/", 1)[1]
        path = directory / name
        if path.exists():
            data = path.read_bytes()
        else:
            with urllib.request.urlopen(artifact["url"], timeout=90) as response:
                data = response.read()
        if digest(data) != artifact["sha256"]:
            raise ValueError(f"{name}: download checksum mismatch")
        path.write_bytes(data)
        if name.endswith(".deb"):
            members = subprocess.check_output(["ar", "t", str(path)], text=True).splitlines()
            member = next(m for m in members if m.startswith("data.tar."))
            payload = subprocess.check_output(["ar", "p", str(path), member])
            with tarfile.open(fileobj=io.BytesIO(payload)) as archive:
                archive.extractall(directory / "root", filter="data")
        print(f"verified {name}", flush=True)


def run(executable, args, *, stdin=None):
    # A regular input file prevents wall-clock-dependent idle tones in minimodem.
    # The caller's environment supplies only runtime-library paths; locale is fixed.
    env = dict(os.environ, LC_ALL="C")
    return subprocess.run(
        [executable, *args], stdin=stdin, capture_output=True,
        check=True, timeout=30, env=env,
    )


def substitute(args, wave):
    return [str(wave) if arg == "{wav}" else arg for arg in args]


def verify_minimodem():
    executable = checked_tool("minimodem")
    manifest = json.loads((DATA / "manifests/independent.json").read_text())
    with tempfile.TemporaryDirectory(prefix="go-rtty-minimodem-") as work:
        for fixture in manifest["fixtures"]:
            name = fixture["id"]
            wave = Path(work) / (name + ".wav")
            with (DATA / fixture["input_path"]).open("rb") as source:
                result = run(executable, substitute(fixture["tx_args"], wave), stdin=source)
            if result.stderr:
                raise ValueError(f"{name}: unexpected transmitter diagnostics: {result.stderr!r}")
            if digest(wave.read_bytes()) != fixture["sha256"]:
                raise ValueError(f"{name}: regenerated WAV differs; record runtime versions before investigating")
            result = run(executable, substitute(fixture["rx_args"], wave))
            if result.stdout.decode("ascii") != fixture["expected_text_usos_on"]:
                raise ValueError(f"{name}: minimodem text mismatch: {result.stdout!r}")
            result = run(executable, ["--binary-output", *substitute(fixture["rx_args"], wave)])
            # minimodem prints each five-bit word least-significant bit first.
            codes = [int(line[::-1], 2) for line in result.stdout.decode("ascii").splitlines()]
            if codes != fixture["expected_codes"]:
                raise ValueError(f"{name}: minimodem raw-code mismatch: {codes}")
            print(f"PASS {name}: identical WAV, exact text, {len(codes)} raw codes", flush=True)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    command = commands.add_parser("fetch", help="download checksum-pinned tools without installing them")
    command.add_argument("directory", type=Path)
    commands.add_parser("verify-minimodem", help="regenerate and decode fixtures without modifying testdata")
    args = parser.parse_args()
    try:
        if args.command == "fetch":
            fetch(args.directory)
        else:
            verify_minimodem()
    except (ValueError, OSError, subprocess.SubprocessError) as error:
        parser.exit(1, f"reference: {error}\n")


if __name__ == "__main__":
    main()
