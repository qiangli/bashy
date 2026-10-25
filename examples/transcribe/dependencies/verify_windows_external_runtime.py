#!/usr/bin/env python3
"""Fail-closed check of an operator-installed Python and native System32 VC files.

The policy hashes are reviewed CI bytes. Passing on an operator host requires
the same bytes; a new installer/runtime update needs explicit pin review.
Neither this check nor its CI evidence authorizes redistribution.
"""

import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys


POLICY = Path(__file__).with_name("windows-external-runtime.json")
VC_DLLS = ("MSVCP140.dll", "VCRUNTIME140.dll")


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def _sha_pin(value):
    return isinstance(value, str) and re.fullmatch(r"[0-9a-f]{64}", value) is not None


def verify(policy, python, version, arch, system_root, version_reader):
    if policy.get("schema") != "s503-windows-external-runtime-v1":
        raise RuntimeError("Windows runtime policy schema is missing or unknown")
    if arch not in ("AMD64", "ARM64") or arch not in policy.get("targets", {}):
        raise RuntimeError("Windows architecture has no reviewed runtime policy")
    target = policy["targets"][arch]
    if not _sha_pin(target.get("python_sha256")) or not re.fullmatch(r"\d+\.\d+\.\d+", target.get("python_version", "")):
        raise RuntimeError("exact Python version and SHA-256 pins are required")
    python = Path(python).resolve(strict=True)
    if not python.is_file() or python.name.lower() != "python.exe":
        raise RuntimeError("an external python.exe file is required")
    if version != target["python_version"]:
        raise RuntimeError(f"Python version {version} does not match reviewed {target['python_version']}")
    if sha256_file(python) != target["python_sha256"]:
        raise RuntimeError("Python executable SHA-256 does not match reviewed pin")
    system32 = (Path(system_root) / "System32").resolve(strict=True)
    runtime = target.get("system32")
    if not isinstance(runtime, dict) or set(runtime) != set(VC_DLLS):
        raise RuntimeError("both reviewed System32 VC runtime pins are required")
    report = {"schema": policy["schema"], "architecture": arch,
              "python": {"path": str(python), "version": version,
                         "sha256": target["python_sha256"]}, "system32": {}}
    for name in VC_DLLS:
        pin = runtime[name]
        if not _sha_pin(pin.get("sha256")) or not re.fullmatch(r"\d+\.\d+\.\d+\.\d+", pin.get("version", "")):
            raise RuntimeError(f"exact {name} version and SHA-256 pins are required")
        dll = (system32 / name).resolve(strict=True)
        if not dll.is_file() or dll.parent != system32:
            raise RuntimeError(f"native System32 VC runtime is absent: {name}")
        if sha256_file(dll) != pin["sha256"]:
            raise RuntimeError(f"{name} SHA-256 does not match reviewed System32 pin")
        actual_version = version_reader(dll)
        if actual_version != pin["version"]:
            raise RuntimeError(f"{name} version does not match reviewed System32 pin")
        report["system32"][name] = {"path": str(dll), **pin}
    return report


def windows_file_version(path):
    env = dict(os.environ, S503_RUNTIME_DLL=str(path))
    output = subprocess.check_output(
        ["powershell.exe", "-NoProfile", "-NonInteractive", "-Command",
         "$v=(Get-Item -LiteralPath $env:S503_RUNTIME_DLL).VersionInfo; "
         "'{0}.{1}.{2}.{3}' -f $v.FileMajorPart,$v.FileMinorPart,$v.FileBuildPart,$v.FilePrivatePart"],
        env=env, text=True, timeout=30).strip()
    if not re.fullmatch(r"\d+\.\d+\.\d+\.\d+", output):
        raise RuntimeError(f"unreadable VC runtime version: {path.name}")
    return output


def main():
    if platform.system() != "Windows" or sys.maxsize <= 2**32:
        raise RuntimeError("native 64-bit Windows Python is required")
    configured = os.environ.get("BASHY_TRANSCRIBE_PYTHON", "")
    if not configured or Path(configured).resolve(strict=True) != Path(sys.executable).resolve(strict=True):
        raise RuntimeError("explicit operator-provided BASHY_TRANSCRIBE_PYTHON must select this interpreter")
    system_root = os.environ.get("SystemRoot")
    if not system_root:
        raise RuntimeError("SystemRoot is absent")
    with POLICY.open(encoding="utf-8") as source:
        policy = json.load(source)
    report = verify(policy, sys.executable, platform.python_version(),
                    platform.machine().upper(), system_root, windows_file_version)
    print(json.dumps(report, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, RuntimeError, subprocess.SubprocessError) as exc:
        sys.exit(f"external Windows runtime rejected: {exc}")
