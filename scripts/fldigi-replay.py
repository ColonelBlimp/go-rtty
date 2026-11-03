#!/usr/bin/env python3
# SPDX-FileCopyrightText: 2026 Marc L. Veary (7Q5MLV)
# SPDX-License-Identifier: GPL-3.0-only
"""Replay fixtures through the pinned, unmodified fldigi application on Xvfb.

This Linux/X11 adapter uses the actual File > Audio > Playback UI and captures
the receive widget through XML-RPC. It is not a headless fldigi CLI or an
extracted modem. The menu geometry is specific to the pinned FLTK build.
"""

import argparse
import base64
from contextlib import contextmanager
import ctypes
import json
import os
from pathlib import Path
import select
import shutil
import socket
import subprocess
import sys
import tempfile
import time
import xmlrpc.client

sys.dont_write_bytecode = True
import reference


class Pointer:
    def __init__(self, display):
        self.x = ctypes.CDLL("libX11.so.6")
        self.xt = ctypes.CDLL("libXtst.so.6")
        ptr = ctypes.c_void_p
        self.x.XOpenDisplay.argtypes = [ctypes.c_char_p]
        self.x.XOpenDisplay.restype = ptr
        self.x.XSync.argtypes = [ptr, ctypes.c_int]
        self.x.XCloseDisplay.argtypes = [ptr]
        self.xt.XTestFakeMotionEvent.argtypes = [ptr, ctypes.c_int, ctypes.c_int, ctypes.c_int, ctypes.c_ulong]
        self.xt.XTestFakeButtonEvent.argtypes = [ptr, ctypes.c_uint, ctypes.c_int, ctypes.c_ulong]
        self.display = self.x.XOpenDisplay(display.encode())
        if not self.display:
            raise RuntimeError("cannot connect to private Xvfb display")

    def move(self, x, y, click=False):
        self.xt.XTestFakeMotionEvent(self.display, 0, x, y, 0)
        if click:
            self.xt.XTestFakeButtonEvent(self.display, 1, 1, 0)
            self.xt.XTestFakeButtonEvent(self.display, 1, 0, 0)
        self.x.XSync(self.display, 0)
        time.sleep(0.35)

    def replay(self):
        # Fixed 800x460 main window at +50+50; chooser cwd contains only fixture.wav.
        self.move(70, 62, True)     # File
        self.move(180, 197)         # Audio submenu
        self.move(265, 250)         # Playback
        self.move(265, 250, True)
        self.move(160, 167, True)   # fixture.wav, immediately after ../
        self.move(440, 450, True)   # Open
        self.move(440, 450, True)   # No continuous loop

    def close(self):
        self.x.XCloseDisplay(self.display)


class Transport(xmlrpc.client.Transport):
    def make_connection(self, host):
        connection = super().make_connection(host)
        connection.timeout = 2
        return connection


def stop(process):
    if process.poll() is None:
        process.terminate()
        try:
            process.wait(timeout=5)
        except subprocess.TimeoutExpired:
            process.kill()
            process.wait(timeout=5)


def configuration(fixture, usos):
    settings = {
        "AUDIOIO": 3, "RTTYSHIFT": 3, "RTTYBAUD": 1, "RTTYBITS": 0,
        "RTTYPARITY": 0, "RTTYSTOP": fixture["stop_half_bits"] - 2,
        "UOSRX": usos, "UOSTX": 0, "RTTYAUTOCRLF": 0, "RTTYCRCLF": 0,
        "RX_LOWERCASE": 0, "KAHNDEMOD": 1, "RTTYBW": 68, "RTTYFILTER": 1.25,
        "RTTYCWI": 0, "PSKREP_AUTOSTART": 0, "USEPSKREP": 0,
        "CHKUSEHAMLIBIS": 0, "CHKUSERIGCATIS": 0, "CHKUSEXMLRPCIS": 0,
    }
    xml = '<?xml version="1.0"?>\n<FLDIGI_DEFS>\n'
    xml += "".join(f"<{key}>{value}</{key}>\n" for key, value in settings.items())
    return xml + "</FLDIGI_DEFS>\n"


def replay(executable, fixture, usos, display, directory, *, source=None, capture_only=False):
    directory.mkdir()
    config = directory / "config"
    config.mkdir()
    home = directory / "home"
    home.mkdir()
    audio = directory / "audio"
    audio.mkdir()
    xml = configuration(fixture, usos)
    (config / "fldigi_def.xml").write_text(xml)
    source = source if source is not None else reference.DATA / fixture["path"]
    if reference.digest(source.read_bytes()) != fixture["sha256"]:
        raise ValueError(f"{fixture['id']}: input checksum mismatch")
    shutil.copyfile(source, audio / "fixture.wav")
    with socket.socket() as reserved:
        reserved.bind(("127.0.0.1", 0))
        port = reserved.getsockname()[1]
    args = [executable, "--home-dir", str(home), "--config-dir", str(config),
            "--xmlrpc-server-address", "127.0.0.1", "--xmlrpc-server-port", str(port),
            "--arq-server-address", "127.0.0.1", "--arq-server-port", "0",
            "--kiss-server-address", "127.0.0.1", "--kiss-server-port-io", "0",
            "-geometry", "800x460+50+50"]
    with (directory / "application.log").open("wb") as log:
        process = subprocess.Popen(args, cwd=audio, stdout=log, stderr=log,
                                   env=dict(os.environ, DISPLAY=display, LC_ALL="C"))
        try:
            with xmlrpc.client.ServerProxy(f"http://127.0.0.1:{port}", transport=Transport()) as rpc:
                deadline = time.monotonic() + 15
                while True:
                    if process.poll() is not None:
                        raise RuntimeError("fldigi exited during startup: " + (directory / "application.log").read_text())
                    try:
                        version = rpc.fldigi.version()
                        break
                    except OSError:
                        if time.monotonic() > deadline:
                            raise RuntimeError("fldigi XML-RPC startup timed out")
                        time.sleep(0.2)
                if Path(rpc.fldigi.config_dir()).resolve() != config.resolve():
                    raise RuntimeError("XML-RPC did not identify the isolated test instance")
                rpc.modem.set_by_name("RTTY")
                rpc.main.rx_only()
                rpc.main.set_afc(False)
                rpc.main.set_squelch(False)
                rpc.main.set_reverse(fixture["mark_hz"] < fixture["space_hz"])
                carrier = fixture.get("carrier_hz", (fixture["mark_hz"] + fixture["space_hz"]) // 2)
                rpc.modem.set_carrier(carrier)
                time.sleep(0.5)
                rpc.text.clear_rx()
                pointer = Pointer(display)
                try:
                    pointer.replay()
                finally:
                    pointer.close()
                deadline = time.monotonic() + fixture["sample_count"] / fixture["sample_rate"] + 1
                while time.monotonic() < deadline:
                    if process.poll() is not None:
                        raise RuntimeError("fldigi exited during playback")
                    time.sleep(min(1, max(0, deadline - time.monotonic())))
                count = rpc.text.get_rx_length()
                raw = rpc.text.get_rx(0, count).data if count else b""
                actual = raw.decode("utf-8")
                # Define the scoring view before comparison: this UI collapses CRLF.
                expected = None if capture_only else fixture["expected_text_usos_on" if usos else "expected_text_usos_off"].replace("\r\n", "\n")
                state = {"modem": rpc.modem.get_name(), "carrier_hz": rpc.modem.get_carrier(),
                         "reverse": bool(rpc.main.get_reverse()), "afc": bool(rpc.main.get_afc()),
                         "squelch": bool(rpc.main.get_squelch())}
                if state != {"modem": "RTTY", "carrier_hz": carrier,
                             "reverse": fixture["mark_hz"] < fixture["space_hz"], "afc": False, "squelch": False}:
                    raise RuntimeError(f"unexpected receiver state: {state}")
                result = {"fixture": fixture["id"], "wav_sha256": fixture["sha256"],
                          "version": version, "rx_usos": bool(usos), "tx_usos": False,
                          "configuration_xml": xml, "configuration_sha256": reference.digest(xml.encode()),
                          "state": state, "expected_widget_text": expected, "actual_widget_text": actual}
                if capture_only:
                    result.update({"status": "captured-unscored", "scored_characters": 0,
                                   "actual_widget_bytes_base64": base64.b64encode(raw).decode("ascii")})
                    print(f"CAPTURED {fixture['id']} USOS={usos}: {len(raw)} widget bytes; unscored", flush=True)
                    return result
                result["pass"] = actual == expected
                if actual != expected:
                    raise RuntimeError(f"{fixture['id']} USOS={usos}: expected {expected!r}, captured {actual!r}")
                print(f"PASS {fixture['id']} USOS={usos}: {actual!r}", flush=True)
                return result
        finally:
            stop(process)


@contextmanager
def display_server(xvfb, work):
    read_fd, write_fd = os.pipe()
    try:
        with (work / "xvfb.log").open("wb") as log:
            server = subprocess.Popen([xvfb, "-displayfd", str(write_fd), "-screen", "0", "1100x850x24",
                                       "-nolisten", "tcp", "-ac"], pass_fds=(write_fd,), stdout=log, stderr=log)
            os.close(write_fd)
            write_fd = None
            try:
                if not select.select([read_fd], [], [], 10)[0]:
                    raise RuntimeError("Xvfb startup timed out")
                number = os.read(read_fd, 32).decode().strip()
                if not number.isdigit() or server.poll() is not None:
                    raise RuntimeError("Xvfb did not provide a display")
                yield ":" + number
            finally:
                stop(server)
    finally:
        os.close(read_fd)
        if write_fd is not None:
            os.close(write_fd)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--output", type=Path, help="write a new result file; existing files are never overwritten")
    parser.add_argument("--fixture", help="run only the named fixture")
    args = parser.parse_args()
    if args.output and args.output.exists():
        parser.error("output already exists")
    executable = reference.checked_tool("fldigi")
    xvfb = os.environ.get("RTTY_XVFB")
    if not xvfb:
        parser.error("set RTTY_XVFB to the Xvfb executable (see docs/REFERENCE.md)")
    manifest = json.loads((reference.DATA / "manifests/independent.json").read_text())
    fixtures = [f for f in manifest["fixtures"] if not args.fixture or f["id"] == args.fixture]
    if not fixtures:
        parser.error("unknown fixture")
    report = {"schema_version": 1, "adapter": "unmodified-application-x11-playback",
              "executable_sha256": reference.digest(Path(executable).read_bytes()), "results": []}
    with tempfile.TemporaryDirectory(prefix="go-rtty-fldigi-") as temporary:
        work = Path(temporary)
        with display_server(xvfb, work) as display:
            for fixture in fixtures:
                for usos in [0, 1]:
                    report["results"].append(replay(executable, fixture, usos, display,
                                                    work / (fixture["id"] + f"-{usos}")))
    if args.output:
        with args.output.open("x") as output:
            json.dump(report, output, indent=2)
            output.write("\n")


if __name__ == "__main__":
    main()
