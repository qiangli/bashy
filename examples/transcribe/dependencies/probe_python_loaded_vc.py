#!/usr/bin/env python3
"""Check the VC modules resolved by the selected installed Windows Python.

This is a pre-admission probe. It deliberately loads each named module in this
Python process, then compares its actual module path and bytes with the
reviewed installed-Python evidence. It does not authorize redistribution.
"""

import ctypes
import hashlib
import json
import os
from pathlib import Path
import platform
import sys


NAMES = ("MSVCP140.dll", "VCRUNTIME140.dll")
OWNERS = {"MSVCP140.dll": "Windows System32",
          "VCRUNTIME140.dll": "Python install directory"}


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def loaded_module_paths():
    kernel = ctypes.WinDLL("kernel32", use_last_error=True)
    kernel.GetModuleFileNameW.argtypes = [ctypes.c_void_p, ctypes.c_wchar_p, ctypes.c_uint32]
    kernel.GetModuleFileNameW.restype = ctypes.c_uint32
    paths = {}
    for name in NAMES:
        # LoadLibrary resolves the name in this interpreter. If it is already
        # loaded, the returned handle identifies the loaded module.
        module = ctypes.WinDLL(name)
        path = ctypes.create_unicode_buffer(32768)
        length = kernel.GetModuleFileNameW(module._handle, path, len(path))
        if not length or length >= len(path) - 1:
            raise RuntimeError(f"loaded module path unavailable: {name}")
        paths[name] = Path(path.value)
    return paths


def verify(policy, arch, python, system_root, loaded):
    if policy.get("schema") != "s503-windows-external-runtime-v1":
        raise RuntimeError("Windows runtime policy schema is missing or unknown")
    if arch not in ("AMD64", "ARM64") or arch not in policy.get("targets", {}):
        raise RuntimeError("Windows architecture has no reviewed runtime policy")
    target = policy["targets"][arch]
    pins = target.get("python_loaded")
    if not isinstance(pins, dict) or set(pins) != set(NAMES) or set(loaded) != set(NAMES):
        raise RuntimeError("both loaded VC modules require reviewed pins and observed paths")
    python = Path(python).resolve(strict=True)
    system32 = (Path(system_root) / "System32").resolve(strict=True)
    expected = {"MSVCP140.dll": system32 / "MSVCP140.dll",
                "VCRUNTIME140.dll": python.parent / "VCRUNTIME140.dll"}
    report = {}
    for name in NAMES:
        pin = pins[name]
        if pin.get("owner") != OWNERS[name] or not isinstance(pin.get("sha256"), str) or \
                len(pin["sha256"]) != 64 or any(c not in "0123456789abcdef" for c in pin["sha256"]):
            raise RuntimeError(f"loaded {name} owner or SHA-256 pin is invalid")
        actual = Path(loaded[name]).resolve(strict=True)
        if actual != expected[name].resolve(strict=True):
            raise RuntimeError(f"{name} loaded from an unreviewed path: {actual}")
        if not actual.is_file() or sha256_file(actual) != pin["sha256"]:
            raise RuntimeError(f"{name} loaded SHA-256 differs from reviewed pin")
        report[name] = {"path": str(actual), "sha256": pin["sha256"]}
    if pins["MSVCP140.dll"]["sha256"] != target["system32"]["MSVCP140.dll"]["sha256"]:
        raise RuntimeError("loaded MSVCP140.dll and System32 pins disagree")
    return report


def main():
    if sys.platform != "win32" or sys.maxsize <= 2**32:
        raise RuntimeError("native 64-bit Windows Python is required")
    if len(sys.argv) != 4:
        raise RuntimeError("policy, architecture and selected interpreter are required")
    policy_path, arch, selected = sys.argv[1:]
    if Path(sys.executable).resolve(strict=True) != Path(selected).resolve(strict=True):
        raise RuntimeError("probe must run under the selected interpreter")
    native = platform.machine().upper()
    if native == "X64":
        native = "AMD64"
    if native != arch:
        raise RuntimeError("interpreter architecture differs from reviewed target")
    with open(policy_path, encoding="utf-8") as source:
        policy = json.load(source)
    if sys.version.split()[0] != policy["targets"][arch]["python_version"]:
        raise RuntimeError("installed interpreter version differs from reviewed patch")
    system_root = os.environ.get("SystemRoot")
    if not system_root:
        raise RuntimeError("SystemRoot is absent")
    report = verify(policy, arch, selected, system_root, loaded_module_paths())
    print(json.dumps({"status": "loaded_vc_verified", "modules": report}, sort_keys=True))


if __name__ == "__main__":
    try:
        main()
    except (OSError, ValueError, KeyError, RuntimeError) as exc:
        sys.exit(f"Windows loaded VC runtime rejected: {exc}")
