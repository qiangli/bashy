#!/usr/bin/env python3
"""Collect bounded, explicitly incomplete evidence for the pinned Deno binary.

This runs only in native CI after the archive and executable have passed their
existing SHA-256 gates. Cargo.lock is a candidate source inventory, not a list
of objects linked into the published executable. The report remains BLOCKED
until a binary-specific linkage and every applicable notice are reviewed.
"""

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import struct
import subprocess
import sys
import tomllib
import urllib.request
import zipfile


SOURCE_COMMIT = "fe7ed012d92c642fd8e5607ac5a45d517538b5c5"
SOURCE_FILES = {
    "LICENSE.md": ("f62497fffecc0852960c8d3e6934b9db86d16396e9b604072e923892cae3a588", 16_384),
    "Cargo.lock": ("36aab78913808a9fb2e5a3a3a27a74a1a1045011133f28beaac6eabf57fc9895", 4_000_000),
    "cli/Cargo.toml": ("ac2ca211c59b910c2b05ecd9ecdd28b3b6b529a29c8002a19695b75e5a669de4", 256_000),
}
NOTICE_NAME = re.compile(r"(^|/)(license|licence|notice|copying)([._-]|$)", re.I)


def sha256(path):
    digest = hashlib.sha256()
    with Path(path).open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def fetch_source(name, expected_sha, limit):
    url = f"https://raw.githubusercontent.com/denoland/deno/{SOURCE_COMMIT}/{name}"
    request = urllib.request.Request(url, headers={"User-Agent": "s503-deno-notice-evidence"})
    with urllib.request.urlopen(request, timeout=60) as response:
        data = response.read(limit + 1)
    if len(data) > limit or hashlib.sha256(data).hexdigest() != expected_sha:
        raise ValueError(f"official source file size or digest drift: {name}")
    return data, url


def command(*args):
    result = subprocess.run(args, capture_output=True, text=True, timeout=30, check=False)
    if result.returncode:
        raise RuntimeError(f"{args[0]} failed with exit {result.returncode}: {result.stderr[:300]}")
    return result.stdout


def provider(path):
    candidate = Path(path)
    if not candidate.is_file():
        return {"path": path, "status": "unresolved"}
    resolved = candidate.resolve()
    return {"path": str(resolved), "sha256": sha256(resolved),
            "bytes": resolved.stat().st_size, "status": "resolved"}


def linux_imports(executable):
    dynamic = command("readelf", "-d", str(executable))
    names = sorted(set(re.findall(r"\(NEEDED\).*?\[([^]]+)\]", dynamic)))
    loaded = command("ldd", str(executable)) if names else ""
    rows = []
    for name in names:
        match = re.search(r"^\s*" + re.escape(name) + r"\s*=>\s*(/\S+)", loaded, re.M)
        rows.append({"name": name, "provider": provider(match.group(1)) if match else
                     {"status": "unresolved"}})
    return {"method": "readelf -d plus ldd", "imports": rows,
            "raw_imports": dynamic[:64_000], "raw_loader_resolution": loaded[:64_000]}


def macos_imports(executable):
    raw = command("otool", "-L", str(executable))
    rows = []
    for line in raw.splitlines()[1:]:
        name = line.strip().split(" (", 1)[0]
        if not name:
            continue
        path = name.replace("@loader_path", str(executable.parent), 1)
        rows.append({"name": name, "provider": provider(path) if path.startswith("/") else
                     {"status": "unresolved", "reason": "dyld token requires runtime resolution"}})
    return {"method": "otool -L", "imports": rows, "raw_imports": raw[:64_000]}


def windows_imports(executable):
    """Read PE import and delay-import names without third-party Python modules."""
    data = executable.read_bytes()
    if len(data) < 0x100 or data[:2] != b"MZ":
        raise ValueError("invalid PE DOS header")
    pe = struct.unpack_from("<I", data, 0x3c)[0]
    if pe + 24 > len(data) or data[pe:pe + 4] != b"PE\0\0":
        raise ValueError("invalid PE signature")
    section_count = struct.unpack_from("<H", data, pe + 6)[0]
    optional_size = struct.unpack_from("<H", data, pe + 20)[0]
    optional = pe + 24
    if optional + optional_size > len(data):
        raise ValueError("truncated PE optional header")
    magic = struct.unpack_from("<H", data, optional)[0]
    if magic not in (0x10b, 0x20b):
        raise ValueError("unknown PE optional header")
    directories = optional + (112 if magic == 0x20b else 96)
    sections = []
    for index in range(section_count):
        offset = optional + optional_size + index * 40
        if offset + 40 > len(data):
            raise ValueError("truncated PE section table")
        virtual_size, virtual_address, raw_size, raw_offset = struct.unpack_from("<IIII", data, offset + 8)
        sections.append((virtual_address, max(virtual_size, raw_size), raw_offset))

    def rva_offset(rva):
        for address, size, raw in sections:
            if address <= rva < address + size:
                offset = raw + rva - address
                if offset < len(data):
                    return offset
        raise ValueError(f"PE RVA outside file: {rva}")

    def cstring(rva):
        start = rva_offset(rva)
        end = data.find(b"\0", start, min(start + 512, len(data)))
        if end < 0:
            raise ValueError("unterminated PE import name")
        return data[start:end].decode("ascii")

    names = set()
    for directory_index, descriptor_size, name_field in ((1, 20, 12), (13, 32, 4)):
        directory = directories + directory_index * 8
        if directory + 8 > optional + optional_size:
            raise ValueError("PE import directory missing")
        rva, size = struct.unpack_from("<II", data, directory)
        if not rva:
            continue
        offset = rva_offset(rva)
        found_end = False
        for position in range(offset, min(offset + size, len(data)), descriptor_size):
            if position + descriptor_size > len(data):
                break
            descriptor = data[position:position + descriptor_size]
            if not any(descriptor):
                found_end = True
                break
            if directory_index == 13 and not (struct.unpack_from("<I", descriptor)[0] & 1):
                raise ValueError("unsupported VA-based PE delay import")
            names.add(cstring(struct.unpack_from("<I", descriptor, name_field)[0]))
        if not found_end:
            raise ValueError("PE import descriptor terminator missing")
    system = Path(os.environ.get("SystemRoot", r"C:\Windows")) / "System32"
    rows = []
    for name in sorted(names, key=str.lower):
        local = executable.parent / name
        system_file = system / name
        resolved = local if local.is_file() else system_file
        rows.append({"name": name, "provider": provider(str(resolved))})
    return {"method": "PE import and delay-import directories", "imports": rows}


def archive_notices(archive, out_dir):
    entries = []
    notices = []
    with zipfile.ZipFile(archive) as package:
        if len(package.infolist()) > 64:
            raise ValueError("Deno archive has unexpectedly many members")
        for member in package.infolist():
            entries.append({"name": member.filename, "bytes": member.file_size,
                            "compressed_bytes": member.compress_size})
            if member.is_dir() or not NOTICE_NAME.search(member.filename):
                continue
            if member.file_size > 1_000_000 or len(notices) >= 16:
                raise ValueError("Deno embedded notice exceeds evidence bound")
            data = package.read(member)
            path = out_dir / f"deno-embedded-notice-{len(notices)}.txt"
            path.write_bytes(data)
            notices.append({"member": member.filename, "artifact": path.name,
                            "sha256": hashlib.sha256(data).hexdigest()})
    return entries, notices


def collect(args, report):
    archive = Path(args.archive)
    executable = Path(args.executable)
    if sha256(archive) != args.archive_sha256 or sha256(executable) != args.executable_sha256:
        raise ValueError("preverified Deno archive or executable bytes drifted")
    report["binary"] = {"archive_sha256": args.archive_sha256,
                        "executable_sha256": args.executable_sha256,
                        "target": args.target}
    report["archive_members"], report["embedded_notices"] = archive_notices(archive, args.out_dir)
    for name, (expected, limit) in SOURCE_FILES.items():
        data, url = fetch_source(name, expected, limit)
        report["source_files"][name] = {"url": url, "sha256": expected, "bytes": len(data)}
        if name == "LICENSE.md":
            (args.out_dir / "deno-source-LICENSE.md").write_bytes(data)
            report["source_files"][name]["artifact"] = "deno-source-LICENSE.md"
        elif name == "Cargo.lock":
            lock = tomllib.loads(data.decode("utf-8"))
        elif name == "cli/Cargo.toml":
            manifest = tomllib.loads(data.decode("utf-8"))
    report["source_build_reference"] = {
        "package": manifest["package"]["name"],
        "version": manifest["package"]["version"],
        "default_features": manifest.get("features", {}).get("default", []),
        "upstream_release_build_features": ["deno/panic-trace"],
        "source": f"https://github.com/denoland/deno/blob/{SOURCE_COMMIT}/.github/workflows/ci.generated.yml",
    }
    if report["source_build_reference"]["version"] != "2.9.8":
        raise ValueError("source manifest version drift")
    packages = []
    for package in lock["package"]:
        packages.append({"name": package["name"], "version": package["version"],
                         "source": package.get("source", "workspace/unknown"),
                         "source_checksum": package.get("checksum"),
                         "license": "UNKNOWN", "linked_to_binary": "UNKNOWN"})
    report["candidate_components"] = packages
    report["candidate_component_scope"] = "entire Cargo.lock; overinclusive and not binary-linked"
    if args.target.startswith("linux/"):
        report["native_imports"] = linux_imports(executable)
    elif args.target.startswith("darwin/"):
        report["native_imports"] = macos_imports(executable)
    elif args.target.startswith("windows/"):
        report["native_imports"] = windows_imports(executable)
    else:
        raise ValueError("unsupported native target")
    unresolved = [row["name"] for row in report["native_imports"]["imports"]
                  if row["provider"]["status"] != "resolved"]
    report["unknowns"] = ["No trusted per-binary link from official release bytes to source components",
                          "Cargo.lock lacks dependency license text and includes unused packages",
                          "V8, zlib-ng and embedded JavaScript/native inputs lack binary-specific notice closure",
                          "Native provider license and redistribution obligations have not been reviewed"]
    if unresolved:
        report["unknowns"].append("Unresolved native imports: " + ", ".join(unresolved))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--archive", required=True)
    parser.add_argument("--archive-sha256", required=True)
    parser.add_argument("--executable", required=True)
    parser.add_argument("--executable-sha256", required=True)
    parser.add_argument("--target", required=True)
    parser.add_argument("--out-dir", type=Path, required=True)
    args = parser.parse_args()
    args.out_dir.mkdir(parents=True, exist_ok=True)
    report = {"schema": "s503-deno-transitive-candidate-v1", "source_commit": SOURCE_COMMIT,
              "source_files": {}, "status": "BLOCKED", "product_admitted": False,
              "unknowns": []}
    try:
        collect(args, report)
    except Exception as error:
        report["collection_error"] = str(error)
        report["unknowns"].append("Evidence collection incomplete")
    finally:
        (args.out_dir / "deno-transitive-candidate.json").write_text(
            json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    print("Deno transitive candidate BLOCKED: " +
          report.get("collection_error", "; ".join(report["unknowns"])), file=sys.stderr)
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
