#!/usr/bin/env python3
"""Record external CI Python and Windows VC runtime identity, without installing either.

The Windows probe reads the native System32 copies that a CLI without app-local
runtime DLLs would find. This is runner evidence, not a redistribution license or
a complete Python distribution audit.
"""

import hashlib
import json
import os
from pathlib import Path
import platform
import re
import subprocess
import sys


VC_DLLS = ("MSVCP140.dll", "VCRUNTIME140.dll")


def sha256_file(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as source:
        for block in iter(lambda: source.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def probe() -> dict:
    python = Path(sys.executable).resolve(strict=True)
    cache = os.environ.get("RUNNER_TOOL_CACHE")
    if not cache:
        raise RuntimeError("GitHub runner tool cache is absent")
    cache_root = Path(cache).resolve(strict=True)
    cache_lexical = Path(os.path.abspath(cache))
    cache_link = None
    if not python.is_relative_to(cache_root):
        # setup-python's macOS framework interpreter reports its real
        # /Library/Frameworks path as sys.executable. Require a launcher in
        # setup-python's selected tool-cache directory that resolves to that
        # exact executable before admitting the external binary.
        location = os.environ.get("pythonLocation", "")
        selected = Path(location).absolute() if location else None
        if (platform.system() != "Darwin" or selected is None or
                not selected.is_relative_to(cache_lexical) or not selected.is_dir()):
            raise RuntimeError("Python is not linked from the GitHub runner tool cache")
        matches = [path for path in (selected / "bin").glob("python*")
                   if path.is_file() and path.resolve(strict=True) == python]
        if not matches:
            raise RuntimeError("Python is not linked from the GitHub runner tool cache")
        cache_link = str(sorted(matches)[0])
    report = {
        "schema": "s503-external-runtime-identity-v1",
        "python": {
            "source": "actions/setup-python@v5 runner tool cache",
            "version": platform.python_version(),
            "path": str(python),
            "tool_cache_link": cache_link,
            "sha256": sha256_file(python),
        },
        "windows_vc_runtime": {},
    }
    if platform.system() == "Windows":
        system_root = os.environ.get("SystemRoot")
        if not system_root:
            raise RuntimeError("SystemRoot is absent")
        system32 = Path(system_root).resolve(strict=True) / "System32"
        for name in VC_DLLS:
            path = (system32 / name).resolve(strict=True)
            if path.parent != system32 or not path.is_file():
                raise RuntimeError(f"native System32 VC runtime is absent: {name}")
            env = dict(os.environ, S503_RUNTIME_DLL=str(path))
            version = subprocess.check_output(
                ["pwsh", "-NoProfile", "-Command",
                 "$v=(Get-Item -LiteralPath $env:S503_RUNTIME_DLL).VersionInfo; "
                 "'{0}.{1}.{2}.{3}' -f $v.FileMajorPart,$v.FileMinorPart,$v.FileBuildPart,$v.FilePrivatePart"],
                env=env, text=True, timeout=30,
            ).strip()
            if not re.fullmatch(r"14\.\d+\.\d+\.\d+", version):
                raise RuntimeError(f"unreadable VC runtime version: {name}: {version!r}")
            report["windows_vc_runtime"][name] = {
                "source": "external Windows System32",
                "version": version,
                "path": str(path),
                "sha256": sha256_file(path),
            }
    return report


if __name__ == "__main__":
    try:
        print(json.dumps(probe(), indent=2))
    except (OSError, RuntimeError, subprocess.SubprocessError) as exc:
        sys.exit(f"runtime identity probe failed closed: {exc}")
