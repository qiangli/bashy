#!/usr/bin/env python3
"""Native, evidence-only Whisper CLI build with Zig's musl/C++ runtime.

Sprint #503 / Story #2742. This probe never marks a dependency admitted.
It intentionally stops at an unknown linker input or missing notice/source.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import shlex
import shutil
import subprocess
import sys
import tarfile
import urllib.request
from pathlib import Path

import build_whisper as pinned


ZIG_VERSION = "0.15.2"
ZIG_SIGNING_KEY = "RWSGOq2NVecA2UPNdBUZykf1CCb147pkmdtYxgb3Ti+JO/wCYvhbAb/U"
ZIG_ARCHIVES = {
    "amd64": ("x86_64", "x86_64-linux-musl", 53733924,
              "02aa270f183da276e5b5920b1dac44a63f1a49e55050ebde3aecc9eb82f93239"),
    "arm64": ("aarch64", "aarch64-linux-musl", 49471996,
              "958ed7d1e00d0ea76590d27666efbf7a932281b3d7ba0c6b01b0ff26498f667f"),
}
SOURCE_ANCHORS = (
    "LICENSE",
    "lib/libc/musl/COPYRIGHT",
    "lib/libc/musl/crt/crt1.c",
    "lib/libc/musl/crt/Scrt1.c",
    "lib/libc/musl/src/env/__libc_start_main.c",
    "lib/libcxx/LICENSE.TXT",
    "lib/libcxxabi/LICENSE.TXT",
    "lib/libunwind/LICENSE.TXT",
    "lib/compiler_rt.zig",
)
FORBIDDEN = re.compile(
    r"(?:^|[/\\(])(?:lib(?:stdc\+\+|supc\+\+|gcc(?:_s)?)"
    r"(?:\.[aso][^/\\() ]*)?|crtbegin[^/\\() ]*\.o|crtend[^/\\() ]*\.o)"
    r"(?:$|[/\\() ])", re.I)
MAP_INPUT = re.compile(r"(?P<path>[^\s()]+\.(?:a|o|obj))(?:\((?P<member>[^()]+)\))?:\(")
MAP_HEADER = re.compile(r"^\s*VMA\s+LMA\s+Size\s+Align\s+Out\s+In\s+Symbol\s*$")
MAP_ROW = re.compile(r"^\s*[0-9a-fA-F]+\s+[0-9a-fA-F]+\s+[0-9a-fA-F]+\s+\d+\s+")
LINK_INPUT_SUFFIXES = (".a", ".o", ".obj")
MUSL_NAMES = {"libc.a", "libm.a", "libpthread.a", "libdl.a", "librt.a",
              "libcrypt.a", "libutil.a", "crt1.o", "Scrt1.o", "rcrt1.o",
              "crti.o", "crtn.o"}
LLVM_NAMES = {"libc++.a": "lib/libcxx/LICENSE.TXT",
              "libc++abi.a": "lib/libcxxabi/LICENSE.TXT",
              "libunwind.a": "lib/libunwind/LICENSE.TXT"}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1 << 20), b""):
            digest.update(block)
    return digest.hexdigest()


def run(command: list[str], record: Path, *, timeout: int = 300) -> str:
    with record.open("w", encoding="utf-8") as stream:
        stream.write("$ " + shlex.join(command) + "\n")
        stream.flush()
        result = subprocess.run(command, stdout=stream, stderr=subprocess.STDOUT,
                                text=True, check=False, timeout=timeout)
    if result.returncode:
        raise RuntimeError(f"command exited {result.returncode}: {record}")
    return record.read_text(encoding="utf-8", errors="replace")


def download(url: str, destination: Path, *, max_bytes: int) -> None:
    request = urllib.request.Request(url, headers={"User-Agent": "s503-linux-musl-probe/1"})
    with urllib.request.urlopen(request, timeout=120) as response, destination.open("wb") as output:
        size = 0
        for block in iter(lambda: response.read(1 << 20), b""):
            size += len(block)
            if size > max_bytes:
                raise RuntimeError(f"download exceeded byte ceiling: {url}")
            output.write(block)


def verified_zig(work: Path, output: Path, arch: str) -> tuple[Path, dict]:
    host, target, expected_size, expected_sha = ZIG_ARCHIVES[arch]
    name = f"zig-{host}-linux-{ZIG_VERSION}.tar.xz"
    url = f"https://ziglang.org/download/{ZIG_VERSION}/{name}"
    archive = work / name
    signature = output / f"{name}.minisig"
    download(url, archive, max_bytes=expected_size)
    download(url + ".minisig", signature, max_bytes=4096)
    if archive.stat().st_size != expected_size or sha256(archive) != expected_sha:
        raise RuntimeError("Zig archive size or SHA-256 differs from pinned official index")
    minisign = shutil.which("minisign")
    if not minisign:
        raise RuntimeError("minisign is required to verify the Zig distribution")
    run([minisign, "-Vm", str(archive), "-x", str(signature), "-P", ZIG_SIGNING_KEY],
        output / "zig-minisign.txt")
    with tarfile.open(archive, "r:xz") as stream:
        stream.extractall(work, filter="data")
    root = work / name.removesuffix(".tar.xz")
    binary = root / "zig"
    if not binary.is_file():
        raise RuntimeError("verified archive lacks expected zig binary")
    notices = output / "runtime-source-anchors"
    notices.mkdir()
    anchors = {}
    for relative in SOURCE_ANCHORS:
        source = root / relative
        if (not source.is_file() or source.stat().st_size == 0 or
                not source.resolve().is_relative_to(root.resolve())):
            raise RuntimeError(f"verified Zig archive lacks source/license anchor {relative}")
        destination = notices / relative
        destination.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(source, destination)
        if sha256(source) != sha256(destination):
            raise RuntimeError(f"source/license anchor changed during retention: {relative}")
        anchors[relative] = {"sha256": sha256(destination), "bytes": destination.stat().st_size}
    version = run([str(binary), "version"], output / "zig-version.txt")
    if version.splitlines()[-1].strip() != ZIG_VERSION:
        raise RuntimeError("Zig binary version does not match archive version")
    run([str(binary), "env"], output / "zig-env.txt")
    return binary, {"url": url, "sha256": expected_sha, "bytes": expected_size,
                    "signature_sha256": sha256(signature), "signing_key": ZIG_SIGNING_KEY,
                    "target": target, "source_anchors": anchors}


def compiler_wrapper(path: Path, zig: Path, language: str, target: str) -> None:
    path.write_text("#!/bin/sh\nexec " + shlex.quote(str(zig)) + " " + language +
                    " -target " + shlex.quote(target) + " \"$@\"\n")
    path.chmod(0o755)


def extract_printed_map(build_log: str, destination: Path) -> None:
    """Retain Zig 0.15's supported --print-map output from the final link."""
    lines = build_log.splitlines(keepends=True)
    headers = [index for index, line in enumerate(lines) if MAP_HEADER.fullmatch(line.strip())]
    if len(headers) != 1:
        raise RuntimeError(f"expected one final LLD --print-map header, found {len(headers)}")
    first = headers[0]
    last = first + 1
    while last < len(lines) and MAP_ROW.match(lines[last]):
        last += 1
    if last - first < 5:
        raise RuntimeError("LLD --print-map lacks input-section rows")
    destination.write_text("".join(lines[first:last]), encoding="utf-8")


def verbose_link_inputs(build_log: str) -> tuple[str, set[str]]:
    """Read the exact ld.lld argv printed by Zig --verbose-link."""
    commands = [line for line in build_log.splitlines()
                if line.startswith("ld.lld ") and re.search(r"(?:^|/)whisper-cli(?:\s|$)", line)]
    if len(commands) != 1:
        raise RuntimeError(f"expected one verbose whisper-cli ld.lld invocation, found {len(commands)}")
    args = shlex.split(commands[0])
    if any(arg.startswith("-l") and arg != "ld.lld" for arg in args):
        raise RuntimeError("verbose LLD invocation has an unresolved -l library")
    inputs = {arg for arg in args if arg.endswith(LINK_INPUT_SUFFIXES)}
    if not inputs:
        raise RuntimeError("verbose LLD invocation has no object or archive inputs")
    return commands[0], inputs


def check_link_inputs(map_file: Path, build_log: str, build: Path,
                      zig_root: Path, zig_cache: Path,
                      retain_dir: Path | None = None) -> dict:
    contents = map_file.read_text(encoding="utf-8", errors="replace")
    if not contents or " Out " not in contents or " In " not in contents:
        raise RuntimeError("missing or unrecognized LLD ELF link map")
    extracted = {(match.group("path"), match.group("member") or "")
                 for match in MAP_INPUT.finditer(contents)}
    link_command, direct = verbose_link_inputs(build_log)
    if not extracted:
        raise RuntimeError("link map did not enumerate input sections")
    allowed_roots = tuple(root.resolve() for root in (build, zig_root, zig_cache))
    inputs = sorted({path for path, _ in extracted} | direct)
    inventory = []
    retained_bytes = 0
    if retain_dir is not None:
        retain_dir.mkdir(parents=True, exist_ok=True)
    for raw in inputs:
        if FORBIDDEN.search(raw):
            raise RuntimeError(f"GCC runtime input in musl link: {raw}")
        path = Path(raw)
        if not path.is_absolute():
            path = build / path
        resolved = path.resolve(strict=True)
        if not any(resolved.is_relative_to(root) for root in allowed_roots):
            raise RuntimeError(f"link input outside verified Zig/build roots: {resolved}")
        if resolved.is_relative_to(build.resolve()):
            category, license_anchor = "whisper-build", "whisper.cpp/LICENSE"
        else:
            name = resolved.name
            if name in MUSL_NAMES:
                category, license_anchor = "zig-musl", "lib/libc/musl/COPYRIGHT"
            elif name in LLVM_NAMES:
                category, license_anchor = "zig-llvm-runtime", LLVM_NAMES[name]
            elif name.startswith(("libcompiler_rt", "compiler_rt", "libclang_rt.builtins")):
                category, license_anchor = "zig-compiler-runtime", "LICENSE"
            else:
                raise RuntimeError(f"unclassified Zig link input; source review required: {resolved}")
        digest = sha256(resolved)
        item = {"path": raw, "resolved_path": str(resolved),
                "sha256": digest, "category": category,
                "license_anchor": license_anchor,
                "members": sorted({member for archive, member in extracted
                                   if archive == raw and member})}
        if retain_dir is not None:
            retained_bytes += resolved.stat().st_size
            if retained_bytes > 512 << 20:
                raise RuntimeError("link input retention exceeds 512 MiB")
            retained = retain_dir / f"{digest}-{resolved.name}"
            if not retained.exists():
                shutil.copyfile(resolved, retained)
            if sha256(retained) != digest:
                raise RuntimeError(f"retained link input differs: {resolved}")
            item["retained_path"] = str(retained.relative_to(retain_dir.parent))
        inventory.append(item)
    return {"map_input_count": len(extracted), "direct_link_input_count": len(direct),
            "verbose_link_command": link_command, "inputs": inventory}


def inspect_elf(binary: Path, output: Path, arch: str) -> dict:
    readelf = shutil.which("readelf")
    nm = shutil.which("nm")
    if not readelf or not nm:
        raise RuntimeError("readelf and nm are required on the native runner")
    outputs = {}
    for name, arguments in {
        "header": ["-h"], "program-headers": ["-l"], "sections": ["-S"],
        "dynamic": ["-d"], "versions": ["--version-info"], "symbols": ["-s"],
    }.items():
        path = output / f"elf-{name}.txt"
        outputs[name] = run([readelf, *arguments, str(binary)], path)
    undefined = run([nm, "-u", str(binary)], output / "elf-undefined.txt")
    machine = {"amd64": "Advanced Micro Devices X86-64", "arm64": "AArch64"}[arch]
    if machine not in outputs["header"]:
        raise RuntimeError("ELF machine differs from native job architecture")
    if re.search(r"\bINTERP\b|Requesting program interpreter", outputs["program-headers"]):
        raise RuntimeError("ELF has a program interpreter")
    if re.search(r"\bNEEDED\b", outputs["dynamic"]):
        raise RuntimeError("ELF has a dynamic library dependency")
    if re.search(r"\b(?:GLIBC|GLIBCXX|GCC|CXXABI)_[\w.]+", outputs["versions"]):
        raise RuntimeError("ELF has GNU runtime version requirements")
    assert_no_undefined_symbols(undefined)
    return {"static_elf": True, "machine": machine,
            "readelf_sha256": {name: sha256(output / f"elf-{name}.txt") for name in outputs},
            "undefined_sha256": sha256(output / "elf-undefined.txt")}


def assert_no_undefined_symbols(nm_record: str) -> None:
    """The first record line is our command; every following byte is nm output."""
    command, separator, output = nm_record.partition("\n")
    if not separator or not command.startswith("$ ") or output.strip():
        raise RuntimeError("ELF has undefined symbols or nm evidence is malformed")


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work-dir", type=Path, required=True)
    parser.add_argument("--output-dir", type=Path, required=True)
    parser.add_argument("--expected-arch", choices=tuple(ZIG_ARCHIVES), required=True)
    parser.add_argument("--parallel", type=int, default=4)
    args = parser.parse_args()
    if not 1 <= args.parallel <= 8:
        parser.error("--parallel must be 1..8")
    os_name, arch, runner = pinned.detect_target()
    if os_name != "linux" or arch != args.expected_arch:
        raise RuntimeError(f"native runner mismatch: expected linux/{args.expected_arch}, got {os_name}/{arch}")
    work = args.work_dir.resolve()
    output = args.output_dir.resolve()
    work.mkdir(parents=True, exist_ok=True)
    output.mkdir(parents=True, exist_ok=True)
    report_path = output / "probe-report.json"
    report = {"schema": "s503-linux-musl-runtime-probe-v1", "platform": f"linux/{arch}",
              "runner_label": runner, "host_machine": platform.machine(),
              "commit_sha": os.environ.get("GITHUB_SHA", ""), "product_admitted": False,
              "probe_passed": False}
    try:
        zig, report["zig"] = verified_zig(work, output, arch)
        target = report["zig"]["target"]
        cache = work / "zig-cache"
        cache.mkdir()
        os.environ["ZIG_GLOBAL_CACHE_DIR"] = str(cache)
        source_archive = work / f"whisper.cpp-{pinned.PINNED_VERSION}.tar.gz"
        pinned.download_source_archive(source_archive)
        pinned.verify_source_archive(source_archive)
        report["whisper_source"] = {"version": pinned.PINNED_VERSION,
                                     "sha256": sha256(source_archive),
                                     "bytes": source_archive.stat().st_size}
        source_parent = work / "source"
        source_parent.mkdir()
        with tarfile.open(source_archive, "r:gz") as stream:
            stream.extractall(source_parent, filter="data")
        source = source_parent / f"whisper.cpp-{pinned.PINNED_VERSION}"
        if not source.is_dir():
            raise RuntimeError("pinned whisper source directory missing")
        wrappers = work / "wrappers"
        wrappers.mkdir()
        cc, cxx = wrappers / "cc", wrappers / "cxx"
        compiler_wrapper(cc, zig, "cc", target)
        compiler_wrapper(cxx, zig, "c++", target)
        build = work / "build"
        map_file = output / "whisper-cli.link.map"
        flags = pinned.get_cmake_build_flags("linux", arch,
                                             {"c_compiler": str(cc), "cxx_compiler": str(cxx)})
        flags = [flag for flag in flags if not flag.startswith("-DCMAKE_EXE_LINKER_FLAGS=")]
        flags.extend(("-DCMAKE_EXPORT_COMPILE_COMMANDS=ON",
                      "-DCMAKE_EXE_LINKER_FLAGS=-static -Wl,--print-map --verbose-link"))
        if any("static-libgcc" in flag or "static-libstdc++" in flag for flag in flags):
            raise RuntimeError("GCC static-runtime flag remained in Zig build")
        report["cmake_flags"] = flags
        run(["cmake", "-G", "Ninja", "-S", str(source), "-B", str(build), *flags],
            output / "configure.log", timeout=300)
        pinned.verify_cmake_portable_cpu(build / "CMakeCache.txt", arch)
        shutil.copyfile(build / "CMakeCache.txt", output / "CMakeCache.txt")
        shutil.copyfile(build / "compile_commands.json", output / "compile_commands.json")
        commands = json.loads((output / "compile_commands.json").read_text())
        if not commands or any("/usr/include" in json.dumps(row) or
                               "-march=native" in json.dumps(row) or
                               str(cc) not in row["command"] and str(cxx) not in row["command"] or
                               not Path(row["file"]).resolve().is_relative_to(source.resolve())
                               for row in commands):
            raise RuntimeError("foreign compiler, host headers or native CPU selection in compile commands")
        source_license = source / "LICENSE"
        if not source_license.is_file() or source_license.stat().st_size == 0:
            raise RuntimeError("whisper.cpp source license missing")
        shutil.copyfile(source_license, output / "whisper-source-LICENSE")
        report["whisper_source"]["license_sha256"] = sha256(output / "whisper-source-LICENSE")
        build_log = run(["cmake", "--build", str(build), "--target", "whisper-cli",
                         "--parallel", str(args.parallel), "--verbose"],
                        output / "build.log", timeout=3000)
        if (output / "build.log").stat().st_size > 32 << 20:
            raise RuntimeError("build log exceeds 32 MiB evidence ceiling")
        final_links = [line for line in build_log.splitlines()
                       if re.search(r"(?:^|\s)-o\s+\S*bin/whisper-cli(?:\s|$)", line)
                       and "-static" in line and "-Wl,--print-map" in line
                       and "--verbose-link" in line]
        if len(final_links) != 1 or str(cxx) not in final_links[0]:
            raise RuntimeError("one exact Zig static final link command was not retained")
        (output / "final-link-command.txt").write_text(final_links[0] + "\n")
        report["final_link_command_sha256"] = sha256(output / "final-link-command.txt")
        extract_printed_map(build_log, map_file)
        binary = build / "bin" / "whisper-cli"
        if not binary.is_file() or not map_file.is_file():
            raise RuntimeError("binary or LLD map missing")
        shutil.copyfile(binary, output / "whisper-cli")
        report["binary_sha256"] = sha256(output / "whisper-cli")
        report["link_map_sha256"] = sha256(map_file)
        report["link_inputs"] = check_link_inputs(map_file, build_log, build, zig.parent,
                                                   cache, output / "link-input-bytes")
        (output / "link-inputs.json").write_text(json.dumps(report["link_inputs"], indent=2) + "\n")
        report["elf"] = inspect_elf(output / "whisper-cli", output, arch)
        run([str(output / "whisper-cli"), "--help"], output / "cli-help.txt", timeout=30)
        report["probe_passed"] = True
    except Exception as exc:
        report["error"] = str(exc)
        raise
    finally:
        report_path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        print(f"s503 Linux musl probe ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
