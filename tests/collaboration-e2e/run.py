#!/usr/bin/env python3
"""Offline, real-service/browser collaboration gate. Exit 2 means BLOCKED."""
import argparse
import json
import hashlib
import os
from pathlib import Path
import platform
import re
import shutil
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request

ROOT = Path(__file__).resolve().parents[2]
NAMES = ("matrix", "whiteboard", "brainstorm")


class Blocked(Exception):
    pass


def require(condition, why):
    if not condition:
        raise Blocked(why)


def ensure(condition, why):
    if not condition:
        raise AssertionError(why)


def command(argv, **kwargs):
    return subprocess.run(argv, check=True, **kwargs)


GO_ENV = dict(os.environ, GOTOOLCHAIN="local", GOPROXY="off", GOSUMDB="off")


def offline_go():
    """Choose an executable Go 1.27+ without invoking toolchain resolution."""
    local = shutil.which("go")
    require(local, "go unavailable")
    candidates = [Path(local)]
    try:
        cache = command([local, "env", "GOMODCACHE"], env=GO_ENV,
                        capture_output=True, text=True).stdout.strip()
        if cache:
            candidates.extend(sorted(Path(cache).glob("golang.org/toolchain@v0.0.1-go*/bin/go"),
                                     reverse=True))
    except (OSError, subprocess.CalledProcessError):
        pass
    for binary in candidates:
        if not binary.is_file() or not os.access(binary, os.X_OK):
            continue
        try:
            version = command([str(binary), "version"], env=GO_ENV,
                              capture_output=True, text=True).stdout.strip()
        except (OSError, subprocess.CalledProcessError):
            continue
        match = re.fullmatch(r"go version go(\d+)\.(\d+)(?:\.\d+)? [^\n]+", version)
        if match and (int(match.group(1)), int(match.group(2))) >= (1, 27):
            return binary
    raise Blocked("Go 1.27+ required offline; no usable local or cached toolchain")


def preflight():
    for program in ("bashy", "node", "python3"):
        require(shutil.which(program), program + " unavailable")
    go_binary = offline_go()
    playwright = os.environ.get("PLAYWRIGHT_MODULE")
    require(playwright and Path(playwright).is_file(), "PLAYWRIGHT_MODULE must name an existing Playwright module")
    try:
        command(["node", "-e", "import(process.argv[1]).then(m=>{if(!m.chromium)process.exit(1)})", playwright],
                capture_output=True, text=True)
    except subprocess.CalledProcessError as exc:
        raise Blocked("Playwright chromium import failed") from exc
    chromium = os.environ.get("CHROMIUM_PATH")
    require(chromium and Path(chromium).is_file() and os.access(chromium, os.X_OK),
            "CHROMIUM_PATH must name an installed executable browser")
    require((ROOT / "examples/matrix-chat/matrix-chat.bsh").is_file(), "Matrix runner absent; integrate #303")
    require((ROOT / "examples/matrix-chat/e2e.py").is_file(), "Matrix E2E absent; integrate #303")
    require((ROOT / "examples/whiteboard/browser-test.mjs").is_file(), "whiteboard browser runner absent")
    require((ROOT / "examples/brainstorm/browser-test.mjs").is_file(), "brainstorm browser runner absent")
    try:
        record = json.loads(command(["bashy", "tool", "show", "tuwunel", "--json"],
                                    capture_output=True, text=True).stdout)
    except (subprocess.CalledProcessError, json.JSONDecodeError) as exc:
        raise Blocked("signed tuwunel tool record unavailable") from exc
    binary = Path(record["cli"]["binary"])
    require(binary.is_file() and os.access(binary, os.X_OK),
            "signed tuwunel is not installed locally; no downloads in this gate")
    machine = {"x86_64": "amd64", "aarch64": "arm64", "arm64": "arm64"}.get(platform.machine())
    system = {"Darwin": "darwin", "Linux": "linux"}.get(platform.system())
    require(machine and system, "no signed tuwunel asset for this platform")
    expected = record["cli"]["managed"]["assets"][system + "/" + machine]["sha256"]
    require(hashlib.sha256(binary.read_bytes()).hexdigest() == expected,
            "local tuwunel differs from the signed pinned asset")
    return go_binary


def build(name, output, go_binary):
    try:
        command([str(go_binary), "build", "-o", str(output), "."],
                cwd=ROOT / "examples" / name / "server", env=GO_ENV)
    except subprocess.CalledProcessError as exc:
        raise RuntimeError(name + " offline build failed") from exc


def probe(endpoint, name):
    request = urllib.request.Request(endpoint["origin"] + endpoint["path"], method="GET")
    with urllib.request.urlopen(request, timeout=5) as response:
        ensure(response.status == 200, name + " health returned " + str(response.status))
        data = json.load(response)
    if name == "matrix":
        ensure("versions" in data, "Matrix versions response invalid")
    elif name == "whiteboard":
        ensure(len(data.get("strokes", [])) == 1, "whiteboard persisted stroke unavailable")
    else:
        ensure(data.get("phase") == "closed", "brainstorm closed session unavailable")


def run():
    go_binary = preflight()
    with tempfile.TemporaryDirectory(prefix="bashy-collab-e2e-") as temp:
        root = Path(temp)
        build("whiteboard", root / "whiteboard", go_binary)
        build("brainstorm", root / "brainstorm", go_binary)
        env = dict(os.environ, COLLAB_E2E_CONTROL_DIR=str(root),
                   WHITEBOARD_BIN=str(root / "whiteboard"), BRAINSTORM_BIN=str(root / "brainstorm"))
        runners = {
            "matrix": ["python3", str(ROOT / "examples/matrix-chat/e2e.py")],
            "whiteboard": ["node", str(ROOT / "examples/whiteboard/browser-test.mjs")],
            "brainstorm": ["node", str(ROOT / "examples/brainstorm/browser-test.mjs")],
        }
        processes = {}
        logs = {}
        try:
            for name in NAMES:
                logs[name] = open(root / (name + ".log"), "w+")
                processes[name] = subprocess.Popen(runners[name], cwd=ROOT, env=env,
                                                   stdout=logs[name], stderr=subprocess.STDOUT)
            endpoints = {}
            deadline = time.monotonic() + 240
            while len(endpoints) != len(NAMES):
                for name, process in processes.items():
                    if process.poll() is not None and name not in endpoints:
                        raise RuntimeError(name + " exited before concurrency barrier")
                    marker = root / (name + ".ready")
                    if name not in endpoints and marker.exists():
                        endpoints[name] = json.loads(marker.read_text())
                if time.monotonic() > deadline:
                    raise TimeoutError("runners did not reach concurrency barrier")
                time.sleep(0.1)
            ports = [int(endpoint["origin"].rsplit(":", 1)[1]) for endpoint in endpoints.values()]
            ensure(len(set(ports)) == 3, "tools did not use distinct ports")
            ensure(all("127.0.0.1" in endpoint["origin"] for endpoint in endpoints.values()),
                    "tool unexpectedly exposed a non-loopback listener")
            for name in NAMES:
                probe(endpoints[name], name)
            print("PASS: three independent live services concurrently on distinct ports/data", flush=True)
            for stopped in NAMES:
                (root / (stopped + ".release")).write_text("release")
                try:
                    code = processes[stopped].wait(timeout=30)
                except subprocess.TimeoutExpired as exc:
                    raise RuntimeError(stopped + " did not stop") from exc
                ensure(code == 0, stopped + " runner failed")
                stopped_down = False
                for _ in range(20):
                    try:
                        probe(endpoints[stopped], stopped)
                    except (urllib.error.URLError, ConnectionError):
                        stopped_down = True
                        break
                    time.sleep(0.1)
                ensure(stopped_down, stopped + " endpoint still responds after stop")
                for survivor in NAMES:
                    if survivor != stopped and processes[survivor].poll() is None:
                        probe(endpoints[survivor], survivor)
                    elif survivor != stopped and NAMES.index(survivor) > NAMES.index(stopped):
                        raise RuntimeError(survivor + " stopped with " + stopped)
                print("PASS: stopped " + stopped + "; remaining services responded", flush=True)
            print("PASS: Matrix real signed service, whiteboard and brainstorm real browsers, concurrent isolation")
        except Exception:
            for name in NAMES:
                (root / (name + ".release")).write_text("release")
            raise
        finally:
            for process in processes.values():
                try:
                    process.wait(timeout=10)
                except subprocess.TimeoutExpired:
                    process.terminate()
                    try:
                        process.wait(timeout=5)
                    except subprocess.TimeoutExpired:
                        process.kill()
                        process.wait()
            for name, log in logs.items():
                log.flush()
                log.seek(0)
                contents = log.read()
                if contents:
                    print("--- " + name + " runner ---\n" + contents, file=sys.stderr)
                log.close()


if __name__ == "__main__":
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--preflight", action="store_true", help="check installed prerequisites only")
    args = parser.parse_args()
    try:
        if args.preflight:
            go_binary = preflight()
            print("PREFLIGHT OK: offline Go " + str(go_binary) + " and browser/service prerequisites")
        else:
            run()
    except Blocked as exc:
        print("BLOCKED: " + str(exc), file=sys.stderr)
        sys.exit(2)
    except Exception as exc:
        print("FAIL: " + str(exc), file=sys.stderr)
        sys.exit(1)
