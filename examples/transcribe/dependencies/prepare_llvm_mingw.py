#!/usr/bin/env python3
"""Prepare byte-pinned native llvm-mingw for the Windows whisper build gate.

Sprint #503 / Story #2742 / Story-ID 74ed924ba244.
The toolchain is a build input, never part of the shipped whisper artifact.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import struct
import subprocess
import sys
import tempfile
import urllib.request
import zipfile
from pathlib import Path, PurePosixPath

VERSION = "20260616"
ASSETS = {
    "amd64": ("x86_64", 187504083, "b9b68a4d276e16fa25802aaba458e4638f64b3884c290aaccdc2d87083b6ca35"),
    "arm64": ("aarch64", 181877750, "312593669435bd0bfc1a43ac3fba23c8b27e0610bade88b2738e5a01702a99ba"),
}


def target_arch() -> str:
    if platform.system() != "Windows":
        raise RuntimeError("native llvm-mingw preparation requires Windows")
    arch = platform.machine().lower()
    if arch in ("amd64", "x86_64"):
        return "amd64"
    if arch in ("arm64", "aarch64"):
        return "arm64"
    raise RuntimeError(f"unsupported Windows architecture: {arch}")


def verified_archive(path: Path, size: int, digest: str) -> None:
    if not path.is_file() or path.stat().st_size != size:
        raise RuntimeError(f"llvm-mingw archive byte count mismatch: {path}")
    h = hashlib.sha256()
    with path.open("rb") as stream:
        for chunk in iter(lambda: stream.read(1024 * 1024), b""):
            h.update(chunk)
    if h.hexdigest() != digest:
        raise RuntimeError(f"llvm-mingw archive SHA-256 mismatch: {path}")


def download_archive(url: str, path: Path, size: int, digest: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, pending = tempfile.mkstemp(prefix=".llvm-mingw-", dir=path.parent)
    try:
        h = hashlib.sha256()
        count = 0
        request = urllib.request.Request(url, headers={"User-Agent": "bashy-s503-native-build/1"})
        with os.fdopen(fd, "wb") as output, urllib.request.urlopen(request, timeout=180) as response:
            for chunk in iter(lambda: response.read(1024 * 1024), b""):
                count += len(chunk)
                if count > size:
                    raise RuntimeError("llvm-mingw download exceeds pinned byte count")
                output.write(chunk)
                h.update(chunk)
        if count != size or h.hexdigest() != digest:
            raise RuntimeError("llvm-mingw download fails pinned size/SHA-256")
        os.replace(pending, path)
    finally:
        Path(pending).unlink(missing_ok=True)


def unpack_verified_archive(archive: Path, destination: Path, folder: str) -> Path:
    root = destination / folder
    if root.is_symlink():
        raise RuntimeError(f"llvm-mingw destination is a symlink: {root}")
    with tempfile.TemporaryDirectory(prefix=".llvm-mingw-extract-", dir=destination) as staging:
        with zipfile.ZipFile(archive) as zf:
            for member in zf.infolist():
                name = PurePosixPath(member.filename)
                if (name.is_absolute() or not name.parts or name.parts[0] != folder
                        or any(part in (".", "..") for part in name.parts)
                        or "\\" in member.filename or ":" in member.filename):
                    raise RuntimeError(f"unsafe llvm-mingw archive member: {member.filename}")
            zf.extractall(staging)
        extracted = Path(staging) / folder
        for name in ("clang.exe", "clang++.exe", "llvm-objdump.exe"):
            if not (extracted / "bin" / name).is_file():
                raise RuntimeError(f"llvm-mingw archive lacks {name}")
        if root.exists():
            shutil.rmtree(root)
        shutil.move(str(extracted), root)
    return root


def pe_machine(path: Path) -> int:
    with path.open("rb") as binary:
        if binary.read(2) != b"MZ":
            raise RuntimeError(f"not a Windows executable: {path}")
        binary.seek(0x3c)
        offset = struct.unpack("<I", binary.read(4))[0]
        binary.seek(offset)
        if binary.read(4) != b"PE\0\0":
            raise RuntimeError(f"invalid Windows executable header: {path}")
        return struct.unpack("<H", binary.read(2))[0]


def prepare(destination: Path, archive: Path | None = None) -> dict:
    arch = target_arch()
    token, size, digest = ASSETS[arch]
    folder = f"llvm-mingw-{VERSION}-ucrt-{token}"
    url = f"https://github.com/mstorsjo/llvm-mingw/releases/download/{VERSION}/{folder}.zip"
    destination.mkdir(parents=True, exist_ok=True)
    archive = archive or destination / f"{folder}.zip"
    if not archive.is_file():
        download_archive(url, archive, size, digest)
    verified_archive(archive, size, digest)
    root = unpack_verified_archive(archive, destination, folder)
    compiler = root / "bin" / "clang.exe"
    machine = pe_machine(compiler)
    if machine != {"amd64": 0x8664, "arm64": 0xaa64}[arch]:
        raise RuntimeError(f"llvm-mingw compiler host PE machine mismatch: 0x{machine:04x}")
    result = subprocess.run([str(compiler), "-dumpmachine"], capture_output=True, text=True, check=False)
    target = result.stdout.strip().lower()
    if (result.returncode != 0 or not target.startswith(token + "-")
            or not ("windows-gnu" in target or "mingw32" in target)):
        raise RuntimeError(f"llvm-mingw compiler target mismatch: {target or result.stderr.strip()}")
    return {
        "schema": "s503-llvm-mingw-toolchain-v1",
        "platform": f"windows/{arch}",
        "version": VERSION,
        "source_url": url,
        "archive_bytes": size,
        "archive_sha256": digest,
        "toolchain_root": str(root.resolve()),
        "compiler_host_pe_machine": f"0x{machine:04x}",
        "compiler_default_target": target,
        "project_license": "ISC (toolchain assembly scripts)",
        "license_sources": [
            "https://github.com/mstorsjo/llvm-mingw/blob/20260616/LICENSE.txt",
            "https://llvm.org/docs/DeveloperPolicy.html#license",
            "https://www.mingw-w64.org/support/#licensing",
        ],
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--destination", type=Path, required=True)
    parser.add_argument("--archive", type=Path)
    parser.add_argument("--report-file", type=Path, required=True)
    args = parser.parse_args()
    try:
        report = prepare(args.destination, args.archive)
        args.report_file.parent.mkdir(parents=True, exist_ok=True)
        args.report_file.write_text(json.dumps(report, indent=2) + "\n")
        print(json.dumps(report, indent=2))
        return 0
    except Exception as exc:
        print(f"llvm-mingw preparation ERROR: {exc}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
