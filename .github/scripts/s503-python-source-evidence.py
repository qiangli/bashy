#!/usr/bin/env python3
"""Bounded, fail-closed evidence for the opt-in S503 Linux Python source build."""

from __future__ import annotations

import argparse
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import platform
import re
import socket
import subprocess
import sys

ROOT = Path(__file__).resolve().parents[2]
SETUP = ROOT / "examples/transcribe/dependencies/setup_python_source.py"
SOURCE = "https://www.python.org/ftp/python/3.12.15/Python-3.12.15.tar.xz"
PIN = {
    "archive": ("Python-3.12.15.tar.xz", 20829824,
                "c2c4321961fab0fb999d66e0cecf521c2ab3994c7992873ea99e306c1094fd5a"),
    "signature": ("Python-3.12.15.tar.xz.asc", None,
                  "e80124b35dabf82264f254dbb291697df96768f6e4ee0e43d7e8f110a9ea50b6"),
    "key": ("thomas-wouters.gpg", None,
            "33c3af79f23a83c2a8a569bfd546601be400cb26dd7eb31f6d5e5b621667c391"),
}
KEY_URL = "https://github.com/Yhg1s.gpg"
KEY_FINGERPRINT = "7169605F62C751356D054A26A821E680E5FA6305"
MAX_BYTES = {"signature": 4096, "key": 65536}
KNOWN_LIBRARY = re.compile(
    r"^(?:ld-linux-(?:x86-64|aarch64)\.so\.[12]|"
    r"lib(?:c|m|pthread|dl|util|rt|resolv|anl)\.so\.[026]|"
    r"libcrypt\.so\.1|lib(?:ssl|crypto)\.so\.3|libz\.so\.1|"
    r"libbz2\.so\.1\.0|liblzma\.so\.5|libsqlite3\.so\.0|"
    r"libffi\.so\.8|libexpat\.so\.1|libuuid\.so\.1|"
    r"libnsl\.so\.2|libtirpc\.so\.3|libncursesw\.so\.6|"
    r"libtinfo\.so\.6|libpanelw\.so\.6|libgcc_s\.so\.1)(?:\.[0-9]+)*$"
)


def require(condition: bool, message: str) -> None:
    if not condition:
        raise RuntimeError(message)


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def load_setup():
    spec = importlib.util.spec_from_file_location("setup_python_source", SETUP)
    require(spec is not None and spec.loader is not None, "source setup module absent")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


def native_target(target: str) -> dict:
    machine = platform.machine().lower()
    actual = {"x86_64": "linux/amd64", "aarch64": "linux/arm64"}.get(machine)
    require(platform.system() == "Linux" and actual == target,
            f"runner is not native {target}: {platform.system()}/{machine}")
    require(sys.maxsize > 2**32, "runner Python is not 64-bit")
    return {"system": platform.system(), "machine": machine,
            "target": actual, "runner_python": sys.version.split()[0]}


def preflight(args) -> dict:
    identity = native_target(args.target)
    setup = load_setup()
    require((setup.VERSION, setup.SOURCE_URL, setup.SOURCE_SHA256,
             setup.SOURCE_BYTES, setup.SIGNATURE_SHA256, setup.RELEASE_KEY) ==
            ("3.12.15", SOURCE, PIN["archive"][2], PIN["archive"][1],
             PIN["signature"][2], KEY_FINGERPRINT),
            "source setup pins differ from independently fixed CI pins")
    return {"schema": "s503-python-source-native-preflight-v1",
            "candidate_commit": args.candidate, "identity": identity,
            "source_setup_sha256": sha256(SETUP), "status": "PASS"}


def inputs(args) -> dict:
    paths = {}
    for kind, (name, expected_bytes, expected_hash) in PIN.items():
        path = args.input_dir / name
        require(path.is_file() and not path.is_symlink(), f"{kind} input absent or symlinked")
        size = path.stat().st_size
        require(size == expected_bytes if expected_bytes is not None else
                0 < size <= MAX_BYTES[kind], f"{kind} input size mismatch")
        digest = sha256(path)
        require(digest == expected_hash, f"{kind} input SHA-256 mismatch")
        paths[kind] = {"url": SOURCE if kind == "archive" else
                       SOURCE + ".asc" if kind == "signature" else KEY_URL,
                       "bytes": size, "sha256": digest}
    # The setup repeats signature verification on private staged copies.
    source = load_setup().verify_source(args.input_dir / PIN["archive"][0],
                                        args.input_dir / PIN["signature"][0],
                                        args.input_dir / PIN["key"][0])
    require(source["release_key_fingerprint"] == KEY_FINGERPRINT,
            "signature signer fingerprint differs from CI pin")
    return {"schema": "s503-python-source-native-inputs-v1", "inputs": paths,
            "gpg_signature": "PASS", "status": "PASS"}


def offline(args) -> dict:
    parent = os.environ["PARENT_NETNS"]
    current = os.readlink("/proc/self/ns/net")
    require(current != parent, "network namespace did not change")
    try:
        with socket.create_connection(("1.1.1.1", 443), timeout=3):
            raise RuntimeError("TCP connection succeeded inside isolated namespace")
    except OSError as error:
        denial = {"errno": error.errno, "reason": str(error)}
    command = [sys.executable, str(SETUP), "--prefix", str(args.prefix),
               "--verify-existing"]
    result = subprocess.run(command, capture_output=True, text=True, timeout=120)
    require(result.returncode == 0,
            "offline --verify-existing failed: " + result.stderr[-2000:])
    verified = json.loads(result.stdout.strip().splitlines()[-1])
    require(verified["status"] == "local_source_build_unadmitted",
            "offline receipt claims unexpected admission status")
    return {"schema": "s503-python-source-native-offline-v1",
            "parent_network_namespace": parent, "isolated_network_namespace": current,
            "tcp_denial": denial, "verify_existing": verified, "status": "PASS"}


def needed(path: Path) -> set[str]:
    output = subprocess.check_output(["readelf", "-d", str(path)], text=True, timeout=30)
    return set(re.findall(r"\(NEEDED\).*?\[([^]]+)\]", output))


def package_for(path: Path) -> dict:
    query = subprocess.check_output(["dpkg-query", "-S", str(path)],
                                    text=True, timeout=30)
    owners = {line.split(": ", 1)[0].split(", ", 1)[0]
              for line in query.splitlines() if ": " in line}
    require(len(owners) == 1, f"library has ambiguous package owner: {path}")
    package = owners.pop()
    version = subprocess.check_output(
        ["dpkg-query", "-W", "-f=${Version}", package], text=True, timeout=30).strip()
    require(bool(version), f"package version absent: {package}")
    copyright_file = Path("/usr/share/doc") / package.split(":", 1)[0] / "copyright"
    require(copyright_file.is_file(), f"package copyright absent: {package}")
    return {"package": package, "version": version,
            "copyright_file": str(copyright_file),
            "copyright_sha256": sha256(copyright_file),
            "license_status": "native_host_policy_review_required"}


def summarize(args) -> dict:
    identity = native_target(args.target)
    receipt_path = args.receipt
    receipt = json.loads(receipt_path.read_text())
    offline_report = json.loads(args.offline.read_text())
    require(offline_report["status"] == "PASS", "offline evidence is incomplete")
    require(receipt["schema"] == "s503-linux-python-source-setup-v1" and
            receipt["status"] == "local_source_build_unadmitted" and
            receipt["target"] == args.target and
            receipt["prefix"] == str(args.prefix), "source receipt identity differs")
    files = receipt["installed_files"]
    executable = files["bin/python3.12"]
    require(offline_report["verify_existing"]["executable_sha256"] ==
            executable["sha256"], "offline executable hash differs")
    require(receipt["runtime"]["version"] == "3.12.15", "Python version differs")
    modules = receipt["runtime"]["modules"]
    require(all(not modules[name] for name in ("readline", "_dbm", "_gdbm")),
            "forbidden Python module present")
    require(all(modules[name] for name in ("_ssl", "_hashlib", "zlib", "bz2", "lzma")),
            "required Python module absent")
    require(set(receipt["extension_modules"]) ==
            {name for name in files if name.endswith(".so")},
            "extension module inventory differs")

    closure = receipt["dynamic_libraries"]
    expected_elfs = {"bin/python3.12"} | set(receipt["extension_modules"])
    require(set(closure) == expected_elfs, "ELF closure inventory incomplete")
    library_records = {}
    for relative, deps in closure.items():
        elf = args.prefix / relative
        sonames = needed(elf)
        names = {Path(path).name for path in deps}
        # A module with no DT_NEEDED is valid and must have an empty closure.
        require(not sonames or deps, f"DT_NEEDED closure absent: {relative}")
        require(all(any(name == soname or name.startswith(soname + ".")
                        for name in names) for soname in sonames),
                f"DT_NEEDED library absent: {relative}")
        for raw_path, record in deps.items():
            path = Path(raw_path)
            require(path.is_absolute() and path.is_file() and
                    path.resolve(strict=True) == path,
                    f"dynamic library path unresolved: {raw_path}")
            require(KNOWN_LIBRARY.fullmatch(path.name) is not None,
                    f"unknown or unsupported dynamic library: {path.name}")
            digest = sha256(path)
            require(record["sha256"] == digest,
                    f"dynamic library hash differs: {path}")
            if raw_path not in library_records:
                library_records[raw_path] = {"sha256": digest, **package_for(path)}
    license_file = args.prefix / "LICENSE.python-3.12.15"
    require(files[license_file.name]["sha256"] == sha256(license_file),
            "Python source LICENSE hash differs")
    return {"schema": "s503-python-source-native-validation-v1",
            "candidate_commit": args.candidate, "identity": identity,
            "source_build_status": receipt["status"],
            "installed_file_count": len(files),
            "installed_executable": executable,
            "source_license_sha256": sha256(license_file),
            "extension_modules": receipt["extension_modules"],
            "runtime_modules": modules,
            "dynamic_libraries": library_records,
            "receipt_sha256": sha256(receipt_path),
            "license_status": "review_required; no product or redistribution admission",
            "validation": "PASS_candidate_only"}


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    commands = parser.add_subparsers(dest="command", required=True)
    first = commands.add_parser("preflight")
    first.add_argument("--target", required=True, choices=("linux/amd64", "linux/arm64"))
    first.add_argument("--candidate", required=True)
    second = commands.add_parser("inputs")
    second.add_argument("--input-dir", required=True, type=Path)
    third = commands.add_parser("offline")
    third.add_argument("--prefix", required=True, type=Path)
    fourth = commands.add_parser("summarize")
    fourth.add_argument("--prefix", required=True, type=Path)
    fourth.add_argument("--target", required=True, choices=("linux/amd64", "linux/arm64"))
    fourth.add_argument("--candidate", required=True)
    fourth.add_argument("--receipt", required=True, type=Path)
    fourth.add_argument("--offline", required=True, type=Path)
    args = parser.parse_args()
    result = {"preflight": preflight, "inputs": inputs, "offline": offline,
              "summarize": summarize}[args.command](args)
    print(json.dumps(result, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
