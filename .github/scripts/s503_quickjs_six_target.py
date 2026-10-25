#!/usr/bin/env python3
"""Retain six-target QuickJS-NG build bytes for independent research review."""

import argparse
import json
import os
from pathlib import Path
import re
import shutil
import sys

import s503_quickjs_native_pilot as pilot


RUNNERS = {
    "linux/amd64": ("Linux", "X64"),
    "linux/arm64": ("Linux", "ARM64"),
    "darwin/amd64": ("macOS", "X64"),
    "darwin/arm64": ("macOS", "ARM64"),
    "windows/amd64": ("Windows", "X64"),
    "windows/arm64": ("Windows", "ARM64"),
}
MAX_RETAINED_BYTES = 128 * 1024 * 1024
MAX_EXTERNAL_HASH_BYTES = 512 * 1024 * 1024


def check_runner(target):
    actual = (os.getenv("RUNNER_OS"), os.getenv("RUNNER_ARCH"))
    pilot.require(actual == RUNNERS[target], "native runner identity mismatch: " + repr(actual))
    return {"runner_os": actual[0], "runner_arch": actual[1],
            "candidate_commit": os.getenv("GITHUB_SHA", ""),
            "run_id": os.getenv("GITHUB_RUN_ID", ""), "target": target}


def qjs_configure_args(source_root, work_dir, evidence_dir, target):
    build_dir = work_dir / "build"
    link_map = (evidence_dir / "qjs-link.map").resolve()
    argv = ["cmake", "-S", str(source_root), "-B", str(build_dir),
            "-DBUILD_SHARED_LIBS=OFF", "-DQJS_BUILD_CLI_WITH_MIMALLOC=OFF",
            "-DQJS_BUILD_CLI_WITH_STATIC_MIMALLOC=OFF"]
    if target.startswith("linux/"):
        argv += ["-DCMAKE_BUILD_TYPE=Release",
                 "-DCMAKE_EXE_LINKER_FLAGS_RELEASE=-Wl,-Map," + str(link_map)]
    elif target.startswith("darwin/"):
        arch = "x86_64" if target.endswith("amd64") else "arm64"
        argv += ["-DCMAKE_BUILD_TYPE=Release", "-DCMAKE_OSX_ARCHITECTURES=" + arch,
                 "-DCMAKE_EXE_LINKER_FLAGS_RELEASE=-Wl,-map," + str(link_map)]
    else:
        generator = ("Visual Studio 17 2022" if target.endswith("amd64")
                     else "Visual Studio 18 2026")
        architecture = "x64" if target.endswith("amd64") else "ARM64"
        argv += ["-G", generator, "-A", architecture,
                 "-DCMAKE_EXE_LINKER_FLAGS_RELEASE=/MAP:" + str(link_map) + " /VERBOSE:LIB"]
    return argv, link_map


def machine_from_binary(path):
    data = path.read_bytes()
    if data.startswith(b"\xcf\xfa\xed\xfe"):
        pilot.require(len(data) >= 8, "truncated Mach-O header")
        cpu = int.from_bytes(data[4:8], "little")
        return {0x01000007: "darwin/amd64", 0x0100000c: "darwin/arm64"}.get(
            cpu, "unknown-Mach-O-" + str(cpu))
    return pilot.machine_from_binary(path)


def verify_link_records(link_map, build_log, target):
    pilot.require(link_map.is_file() and link_map.stat().st_size > 0,
                  "native linker map missing")
    if target.startswith("darwin/"):
        text = link_map.read_text(encoding="utf-8", errors="replace")
        pilot.require("# Object files:" in text and "# Symbols:" in text,
                      "Apple linker map lacks object and symbol sections")
    else:
        pilot.verify_link_records(link_map, build_log,
                                  "linux/amd64" if target.startswith("linux/")
                                  else "windows/arm64")


def build_qjs(source_root, work_dir, evidence_dir, target):
    build_dir = work_dir / "build"
    argv, link_map = qjs_configure_args(source_root, work_dir, evidence_dir, target)
    pilot.run_logged(argv, evidence_dir / "configure.log")
    cache = (build_dir / "CMakeCache.txt").read_text(encoding="utf-8")
    for setting in ("BUILD_SHARED_LIBS:BOOL=OFF", "QJS_BUILD_CLI_WITH_MIMALLOC:BOOL=OFF",
                    "QJS_BUILD_CLI_WITH_STATIC_MIMALLOC:BOOL=OFF"):
        pilot.require(setting in cache, "CMake option not enforced: " + setting)
    if target.startswith("darwin/"):
        architecture = "x86_64" if target.endswith("amd64") else "arm64"
        pilot.require("CMAKE_OSX_ARCHITECTURES:UNINITIALIZED=" + architecture in cache or
                      "CMAKE_OSX_ARCHITECTURES:STRING=" + architecture in cache,
                      "CMake architecture option not enforced")
    build_log = evidence_dir / "build.log"
    pilot.run_logged(["cmake", "--build", str(build_dir), "--target", "qjs_exe",
                      "--config", "Release", "--parallel", "2"], build_log)
    name = "qjs.exe" if target.startswith("windows/") else "qjs"
    binaries = [p for p in build_dir.rglob(name) if p.is_file()]
    pilot.require(len(binaries) == 1, "expected one native qjs executable")
    binary = binaries[0]
    pilot.require(machine_from_binary(binary) == target, "qjs architecture mismatch")
    verify_link_records(link_map, build_log, target)
    pilot.run_logged([str(binary), "--version"], evidence_dir / "qjs-version.log")
    pilot.require("\n0.17.0\n" in (evidence_dir / "qjs-version.log").read_text(encoding="utf-8"),
                  "qjs version output did not identify pinned version")
    return binary


def native_imports(binary, evidence_dir, target):
    if target.startswith("linux/"):
        command = ["readelf", "-d", str(binary)]
    elif target.startswith("darwin/"):
        command = ["otool", "-L", str(binary)]
    else:
        dumpbin = shutil.which("dumpbin")
        llvm = shutil.which("llvm-readobj")
        if not dumpbin:
            for key in ("ProgramFiles", "ProgramFiles(x86)"):
                prefix = os.getenv(key)
                if prefix:
                    arch = "x64" if target.endswith("amd64") else "arm64"
                    candidates = sorted(Path(prefix).glob(
                        "Microsoft Visual Studio/*/*/VC/Tools/MSVC/*/bin/Host*/" + arch +
                        "/dumpbin.exe"))
                    if candidates:
                        dumpbin = str(candidates[-1])
                        break
        pilot.require(dumpbin or llvm, "no PE import reader on native runner")
        command = ([dumpbin, "/DEPENDENTS", str(binary)] if dumpbin else
                   [llvm, "--coff-imports", str(binary)])
    log = evidence_dir / "native-imports.log"
    pilot.run_logged(command, log)
    pilot.require(log.stat().st_size > 100, "native import record empty")
    return {"command": command[:1], "sha256": pilot.sha256_file(log),
            "provider_license_review": "PENDING"}


def copy_review_bytes(source_archive, source_root, binary, build_dir, evidence_dir):
    """Copy only QuickJS-produced build bytes, never a host SDK or CRT input."""
    items = [(source_archive, Path("source") / source_archive.name),
             (binary, Path("native") / binary.name)]
    for path in sorted(build_dir.rglob("*")):
        if path.is_file() and is_quickjs_output(path.relative_to(build_dir)):
            items.append((path, Path("objects") / path.relative_to(build_dir)))
    pilot.require(len(items) > 2, "no native object or archive bytes produced")
    total = 0
    inventory = []
    for source, relative in items:
        pilot.require(not source.is_symlink(), "build artifact is a symlink")
        total += source.stat().st_size
        pilot.require(total <= MAX_RETAINED_BYTES, "retained native bytes exceed cap")
        destination = evidence_dir / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copy2(source, destination)
        digest = pilot.sha256_file(source)
        pilot.require(pilot.sha256_file(destination) == digest,
                      "retained copy differs from inspected artifact")
        inventory.append({"path": relative.as_posix(), "bytes": source.stat().st_size,
                          "sha256": digest, "mode": oct(source.stat().st_mode & 0o777)})
    notice_inventory = pilot.record_source_notices(source_root)
    pilot.require(any(item["path"] == "LICENSE" for item in notice_inventory),
                  "QuickJS source notice missing")
    return inventory, notice_inventory


def is_quickjs_output(relative):
    target_dirs = {"qjs.dir", "qjs-libc.dir", "qjs_exe.dir"}
    if relative.suffix.lower() in (".o", ".obj"):
        return bool(target_dirs.intersection(relative.parts))
    if relative.suffix.lower() in (".a", ".lib"):
        if relative.name.lower() in {"libqjs.a", "libqjs-libc.a"}:
            return relative.parent == Path(".")
        if relative.name.lower() in {"qjs.lib", "qjs-libc.lib"}:
            return relative.parent == Path("Release")
    return False


def external_provider_candidate(target):
    if target.startswith("linux/"):
        return "GNU libc/GCC or Linux distribution; terms and notices unreviewed"
    if target.startswith("darwin/"):
        return "Apple SDK/macOS provider; terms and notices unreviewed"
    return "Microsoft Visual Studio/Windows SDK/VC runtime; terms and notices unreviewed"


def external_notice_candidate(target):
    if target.startswith("linux/"):
        return "/usr/share/doc/<owning-package>/copyright; package ownership unverified"
    if target.startswith("darwin/"):
        return "Apple macOS/Xcode SDK license; exact linked provider unverified"
    return "Microsoft Visual Studio/Windows SDK redistributable notices; exact component unverified"


def external_link_inputs(link_map, build_log, imports_log, build_dir, target):
    """Hash on-disk external link candidates without copying provider bytes.

    GNU LOAD records and Apple map entries are linker observations. MSVC
    /VERBOSE:LIB records searched libraries, not a loaded-member closure.
    Native imports are retained separately for the independent reviewer.
    """
    if target.startswith("linux/"):
        observations = [(line[5:].strip(), "GNU ld LOAD") for line in
                        link_map.read_text(encoding="utf-8", errors="replace").splitlines()
                        if line.startswith("LOAD ")]
    elif target.startswith("darwin/"):
        observations = []
        for line in link_map.read_text(encoding="utf-8", errors="replace").splitlines():
            found = re.match(r"^\[\s*\d+\]\s+(.+)$", line.strip())
            if found:
                observations.append((found.group(1).strip(), "Apple ld object map"))
        for line in imports_log.read_text(encoding="utf-8", errors="replace").splitlines():
            value = line.strip().split(" (compatibility version", 1)[0]
            if value.startswith(("/", "@")):
                observations.append((value, "otool direct import"))
    else:
        observations = []
        for line in build_log.read_text(encoding="utf-8", errors="replace").splitlines():
            found = re.search(r"\bSearching\s+(.+?\.lib):?\s*$", line, re.IGNORECASE)
            if found:
                observations.append((found.group(1), "MSVC searched library; not loaded-member proof"))
    seen = set()
    inventory = []
    hashed_bytes = 0
    for raw, relation in observations:
        if raw in seen:
            continue
        seen.add(raw)
        pilot.require(len(seen) <= 256, "external input inventory exceeds bound")
        path = Path(raw)
        if not path.is_absolute():
            path = build_dir / path
        resolved = path.resolve()
        if resolved == build_dir.resolve() or build_dir.resolve() in resolved.parents:
            continue
        record = {"link_path": raw, "resolved_path": str(resolved),
                  "relation": relation, "provider_candidate": external_provider_candidate(target),
                  "notice_candidate": external_notice_candidate(target),
                  "notice_review": "PENDING"}
        if resolved.is_file():
            hashed_bytes += resolved.stat().st_size
            pilot.require(hashed_bytes <= MAX_EXTERNAL_HASH_BYTES,
                          "external input hash volume exceeds bound")
            record["sha256"] = pilot.sha256_file(resolved)
            record["bytes"] = resolved.stat().st_size
        else:
            record["hash_status"] = "not present as a runner file; provider resolution pending"
        inventory.append(record)
    pilot.require(inventory, "native external link observations missing")
    return inventory


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, choices=tuple(RUNNERS))
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--evidence-dir", required=True, type=Path)
    args = parser.parse_args()
    args.work_dir = args.work_dir.resolve()
    args.evidence_dir = args.evidence_dir.resolve()
    args.work_dir.mkdir(parents=True, exist_ok=True)
    args.evidence_dir.mkdir(parents=True, exist_ok=True)
    report = {"schema": "s503-quickjs-six-target-research-v1", "target": args.target,
              "source_commit": pilot.SOURCE_COMMIT, "source_sha256": pilot.SOURCE_SHA256,
              "research_result": "BLOCKED", "product_admitted": False,
              "native_link_notice_closure": "BLOCKED"}
    try:
        report["identity"] = check_runner(args.target)
        source_archive = args.work_dir / "quickjs-source.tar.gz"
        pilot.fetch_exact(pilot.SOURCE_URL, source_archive,
                          pilot.SOURCE_BYTES, pilot.SOURCE_SHA256)
        source_root = pilot.unpack_source(source_archive, args.work_dir / "source")
        binary = build_qjs(source_root, args.work_dir, args.evidence_dir, args.target)
        report["binary"] = {"sha256": pilot.sha256_file(binary),
                            "bytes": binary.stat().st_size,
                            "machine": machine_from_binary(binary)}
        report["retained_bytes"], report["source_notices"] = copy_review_bytes(
            source_archive, source_root, binary, args.work_dir / "build", args.evidence_dir)
        report["native_imports"] = native_imports(binary, args.evidence_dir, args.target)
        report["link_map_sha256"] = pilot.sha256_file(args.evidence_dir / "qjs-link.map")
        report["external_link_inputs"] = external_link_inputs(
            args.evidence_dir / "qjs-link.map", args.evidence_dir / "build.log",
            args.evidence_dir / "native-imports.log", args.work_dir / "build", args.target)
        zipapp = args.work_dir / "yt-dlp-2026.10.21.pyz"
        pilot.fetch_exact(pilot.ZIPAPP_URL, zipapp, pilot.ZIPAPP_BYTES,
                          pilot.ZIPAPP_SHA256)
        report["zipapp_sha256"] = pilot.ZIPAPP_SHA256
        report["ejs_smoke"] = pilot.smoke_ejs(binary, zipapp, args.evidence_dir)
        report["research_result"] = "PASS"
        report["native_link_notice_closure"] = (
            "BLOCKED: retained bytes need independent loaded-input and notice review")
        return 0
    except Exception as exc:
        report["error"] = str(exc)
        return 1
    finally:
        pilot.write_report(args.evidence_dir / "result.json", report)


if __name__ == "__main__":
    sys.exit(main())
