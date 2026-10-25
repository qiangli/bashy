#!/usr/bin/env python3
"""Evidence-only native QuickJS-NG/EJS pilot. Never marks a product admitted."""

import argparse
import hashlib
import json
import os
from pathlib import Path, PurePosixPath
import re
import shutil
import struct
import subprocess
import sys
import tarfile
import urllib.request
import zipfile


SOURCE_COMMIT = "6d46d07d04041b40f4f49eaa7fdebe44c314c699"
SOURCE_URL = "https://codeload.github.com/quickjs-ng/quickjs/tar.gz/" + SOURCE_COMMIT
SOURCE_BYTES = 905853
SOURCE_SHA256 = "a62cf1ff7d6d2f82b90a2d247a57e9eb56b81c03feb1f372a53923426e358cb0"
LICENSE_SHA256 = "96f73f9d2a16c21a36b418f06073be26e7d6d5e7c1bc99756b21a4f2c74ef171"
CMAKE_SHA256 = "32ff0d270bb61887786adef62a1507c740518570d90edd582376d469779313a1"
ZIPAPP_URL = "https://github.com/yt-dlp/yt-dlp/releases/download/2026.10.21/yt-dlp"
ZIPAPP_BYTES = 3074818
ZIPAPP_SHA256 = "b7af77f686e95f7845ce8d09d21957718febb8b48b30555adb9dd2292ad7bf14"
EJS_SHA256 = {
    "yt_dlp_ejs/yt/solver/core.min.js": "18da6ce0758b416e7ae645084f4f8801f9f9d59d6c477c05eaa0ff94ebd8cc00",
    "yt_dlp_ejs/yt/solver/lib.min.js": "c55987fe697e5b9ee18830163f7af85327e9bb5c3e674b969d38c8d205eaa577",
}
SOURCE_PREFIX = "quickjs-" + SOURCE_COMMIT


def sha256_bytes(data):
    return hashlib.sha256(data).hexdigest()


def sha256_file(path):
    digest = hashlib.sha256()
    with path.open("rb") as stream:
        for block in iter(lambda: stream.read(1024 * 1024), b""):
            digest.update(block)
    return digest.hexdigest()


def require(condition, message):
    if not condition:
        raise RuntimeError(message)


def fetch_exact(url, path, expected_size, expected_sha256):
    request = urllib.request.Request(url, headers={"User-Agent": "s503-quickjs-native-pilot"})
    with urllib.request.urlopen(request, timeout=90) as source, path.open("wb") as output:
        total = 0
        while True:
            block = source.read(1024 * 1024)
            if not block:
                break
            total += len(block)
            require(total <= expected_size, "oversize upstream asset: " + url)
            output.write(block)
    require(total == expected_size, "upstream asset size drift: " + url)
    require(sha256_file(path) == expected_sha256, "upstream asset SHA-256 drift: " + url)


def unpack_source(archive, output):
    """Copy only regular files under the pinned prefix, with no tar link escape."""
    root = output / SOURCE_PREFIX
    with tarfile.open(archive, "r:gz") as bundle:
        members = bundle.getmembers()
        require(len(members) > 20, "incomplete QuickJS source archive")
        for member in members:
            parts = PurePosixPath(member.name).parts
            require(parts and parts[0] == SOURCE_PREFIX, "unexpected archive root")
            require(".." not in parts and not member.name.startswith("/"), "unsafe archive path")
            require(member.isdir() or member.isfile(), "archive contains a link or special file")
            destination = output.joinpath(*parts)
            if member.isdir():
                destination.mkdir(parents=True, exist_ok=True)
                continue
            destination.parent.mkdir(parents=True, exist_ok=True)
            with bundle.extractfile(member) as source, destination.open("wb") as target:
                shutil.copyfileobj(source, target)
    license_file = root / "LICENSE"
    cmake_file = root / "CMakeLists.txt"
    require(sha256_file(license_file) == LICENSE_SHA256, "QuickJS license changed")
    require(sha256_file(cmake_file) == CMAKE_SHA256, "QuickJS CMake recipe changed")
    require(license_file.read_text(encoding="utf-8").startswith("The MIT License (MIT)"),
            "QuickJS license text changed")
    return root


def record_source_notices(source_root):
    names = ("LICENSE", "LICENCE", "COPYING", "NOTICE")
    return [
        {"path": str(path.relative_to(source_root)), "sha256": sha256_file(path)}
        for path in sorted(source_root.rglob("*"))
        if path.is_file() and any(path.name.upper().startswith(name) for name in names)
    ]


def run_logged(argv, log_path, cwd=None):
    with log_path.open("w", encoding="utf-8") as output:
        output.write("argv=" + json.dumps(argv) + "\n")
        output.flush()
        result = subprocess.run(argv, cwd=cwd, stdout=output, stderr=subprocess.STDOUT,
                                text=True, check=False)
        output.write("\nexit_code=" + str(result.returncode) + "\n")
    require(result.returncode == 0, "command failed: " + str(log_path))


def machine_from_binary(path):
    data = path.read_bytes()
    if data.startswith(b"\x7fELF"):
        require(len(data) >= 20 and data[4] == 2, "ELF is not 64-bit")
        machine = struct.unpack_from("<H", data, 18)[0]
        return {62: "linux/amd64", 183: "linux/arm64"}.get(machine, "unknown-ELF-" + str(machine))
    if data.startswith(b"MZ"):
        require(len(data) >= 0x40, "truncated PE DOS header")
        offset = struct.unpack_from("<I", data, 0x3c)[0]
        require(offset + 6 <= len(data) and data[offset:offset + 4] == b"PE\0\0",
                "invalid PE header")
        machine = struct.unpack_from("<H", data, offset + 4)[0]
        return {0x8664: "windows/amd64", 0xaa64: "windows/arm64"}.get(
            machine, "unknown-PE-" + str(machine))
    return "unknown-format"


def build_qjs(source_root, work_dir, evidence_dir, target):
    build_dir = work_dir / "build"
    link_map = evidence_dir / "qjs-link.map"
    if target == "linux/amd64":
        argv = ["cmake", "-S", str(source_root), "-B", str(build_dir),
                "-DCMAKE_BUILD_TYPE=Release", "-DBUILD_SHARED_LIBS=OFF",
                "-DQJS_BUILD_CLI_WITH_MIMALLOC=OFF",
                "-DQJS_BUILD_CLI_WITH_STATIC_MIMALLOC=OFF",
                "-DCMAKE_EXE_LINKER_FLAGS_RELEASE=-Wl,-Map," + str(link_map)]
    elif target == "windows/arm64":
        argv = ["cmake", "-S", str(source_root), "-B", str(build_dir),
                "-G", "Visual Studio 17 2022", "-A", "ARM64",
                "-DBUILD_SHARED_LIBS=OFF", "-DQJS_BUILD_CLI_WITH_MIMALLOC=OFF",
                "-DQJS_BUILD_CLI_WITH_STATIC_MIMALLOC=OFF",
                "-DCMAKE_EXE_LINKER_FLAGS_RELEASE=/MAP:" + str(link_map) + " /VERBOSE:LIB"]
    else:
        raise RuntimeError("pilot target is outside the two-runner scope")
    run_logged(argv, evidence_dir / "configure.log")
    cache = (build_dir / "CMakeCache.txt").read_text(encoding="utf-8")
    for setting in ("BUILD_SHARED_LIBS:BOOL=OFF", "QJS_BUILD_CLI_WITH_MIMALLOC:BOOL=OFF",
                    "QJS_BUILD_CLI_WITH_STATIC_MIMALLOC:BOOL=OFF"):
        require(setting in cache, "CMake option not enforced: " + setting)
    run_logged(["cmake", "--build", str(build_dir), "--target", "qjs_exe",
                "--config", "Release", "--parallel", "2"], evidence_dir / "build.log")
    binaries = [path for path in build_dir.rglob("qjs.exe" if target.startswith("windows") else "qjs")
                if path.is_file()]
    require(len(binaries) == 1, "expected one native qjs executable")
    require(machine_from_binary(binaries[0]) == target, "qjs architecture mismatch")
    require(link_map.is_file() and link_map.stat().st_size > 0, "native linker map missing")
    run_logged([str(binaries[0]), "--version"], evidence_dir / "qjs-version.log")
    require("\n0.17.0\n" in (evidence_dir / "qjs-version.log").read_text(encoding="utf-8"),
            "qjs version output did not identify pinned version")
    objects = []
    for path in sorted(build_dir.rglob("*")):
        if path.is_file() and path.suffix.lower() in (".o", ".obj", ".a", ".lib"):
            objects.append({"path": str(path.relative_to(build_dir)), "sha256": sha256_file(path)})
    require(objects, "build objects were not retained")
    return binaries[0], link_map, objects


def native_imports(binary, evidence_dir, target):
    if target.startswith("linux"):
        command = ["readelf", "-d", str(binary)]
    else:
        dumpbin = shutil.which("dumpbin")
        llvm = shutil.which("llvm-readobj")
        if not dumpbin:
            for key in ("ProgramFiles", "ProgramFiles(x86)"):
                prefix = os.getenv(key)
                if not prefix:
                    continue
                candidates = sorted(Path(prefix).glob(
                    "Microsoft Visual Studio/*/*/VC/Tools/MSVC/*/bin/Host*/arm64/dumpbin.exe"))
                if candidates:
                    dumpbin = str(candidates[-1])
                    break
        require(dumpbin or llvm, "no PE import reader on native runner")
        command = ([dumpbin, "/DEPENDENTS", str(binary)] if dumpbin else
                   [llvm, "--coff-imports", str(binary)])
    log = evidence_dir / "native-imports.log"
    run_logged(command, log)
    require(log.stat().st_size > 100, "native import record empty")
    return {"command": command[:1], "sha256": sha256_file(log),
            "provider_license_review": "PENDING"}


def read_pinned_ejs(zipapp):
    with zipfile.ZipFile(zipapp) as archive:
        version = archive.read("yt_dlp_ejs/_version.py").decode("utf-8")
        require(re.search(r"__version__\s*=\s*version\s*=\s*'0\.8\.0'", version),
                "bundled EJS version drift")
        scripts = {}
        for name, expected in EJS_SHA256.items():
            data = archive.read(name)
            require(sha256_bytes(data) == expected, "bundled EJS SHA-256 drift: " + name)
            scripts[name] = data.decode("utf-8")
    return scripts


def smoke_ejs(binary, zipapp, evidence_dir):
    scripts = read_pinned_ejs(zipapp)
    run_logged([sys.executable, "-I", str(zipapp), "--ignore-config",
                "--no-remote-components", "--no-js-runtimes",
                "--js-runtimes", "quickjs:" + str(binary), "--version"],
               evidence_dir / "yt-dlp-options.log")
    require("2026.10.21" in (evidence_dir / "yt-dlp-options.log").read_text(encoding="utf-8"),
            "yt-dlp version or runtime-option parse mismatch")
    # This exercises the pinned bundled EJS dispatch with a deterministic
    # prepared solver. Player extraction and live YouTube remain unproved.
    prepared = "_result.sig=function(x){return x.split('').reverse().join('')};" + \
               "_result.n=function(x){return x+'n'};"
    request = {"type": "preprocessed", "preprocessed_player": prepared,
               "requests": [{"type": "sig", "challenges": ["abc"]},
                            {"type": "n", "challenges": ["xyz"]}]}
    js_file = evidence_dir / "controlled-ejs.js"
    js_file.write_text(scripts["yt_dlp_ejs/yt/solver/lib.min.js"] + "\n" +
                       "Object.assign(globalThis, lib);\n" +
                       scripts["yt_dlp_ejs/yt/solver/core.min.js"] + "\n" +
                       "console.log(JSON.stringify(jsc(" + json.dumps(request) + ")));\n",
                       encoding="utf-8")
    result = subprocess.run([str(binary), "--script", str(js_file)], capture_output=True,
                            text=True, check=False, timeout=30)
    (evidence_dir / "controlled-ejs-output.log").write_text(
        result.stdout + "\nSTDERR\n" + result.stderr, encoding="utf-8")
    require(result.returncode == 0, "QuickJS failed bundled EJS dispatch")
    lines = [line for line in result.stdout.splitlines() if line.strip()]
    require(lines, "QuickJS EJS produced no response")
    response = json.loads(lines[-1])
    require(response.get("type") == "result" and
            response.get("responses") == [
                {"type": "result", "data": {"abc": "cba"}},
                {"type": "result", "data": {"xyz": "xyzn"}}],
            "QuickJS EJS controlled solver mismatch")
    return {"result": "PASS", "scope": "bundled EJS prepared-solver dispatch only",
            "player_extraction": "NOT_RUN", "youtube_acquisition": "NOT_RUN",
            "remote_ejs_components": "DISABLED; scripts came from hash-verified zipapp"}


def check_runner(target):
    want = {"linux/amd64": ("Linux", "X64"), "windows/arm64": ("Windows", "ARM64")}
    require(target in want, "target outside this pilot")
    actual = (os.getenv("RUNNER_OS"), os.getenv("RUNNER_ARCH"))
    require(actual == want[target], "runner identity mismatch: " + repr(actual))
    return {"runner_os": actual[0], "runner_arch": actual[1],
            "candidate_commit": os.getenv("GITHUB_SHA", ""), "expected_target": target}


def write_report(path, report):
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(json.dumps(report, indent=2, sort_keys=True) + "\n", encoding="utf-8")


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--target", required=True, choices=("linux/amd64", "windows/arm64"))
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--evidence-dir", required=True, type=Path)
    args = parser.parse_args()
    args.work_dir.mkdir(parents=True, exist_ok=True)
    args.evidence_dir.mkdir(parents=True, exist_ok=True)
    report = {"schema": "s503-quickjs-native-pilot-v1", "product_admitted": False,
              "research_result": "BLOCKED", "native_link_notice_closure": "BLOCKED",
              "source_commit": SOURCE_COMMIT, "source_sha256": SOURCE_SHA256,
              "license": "MIT source claim; executable obligations unreviewed",
              "target": args.target}
    try:
        report["identity"] = check_runner(args.target)
        source_archive = args.work_dir / "quickjs-source.tar.gz"
        fetch_exact(SOURCE_URL, source_archive, SOURCE_BYTES, SOURCE_SHA256)
        source_root = unpack_source(source_archive, args.work_dir / "source")
        report["source_notices"] = record_source_notices(source_root)
        binary, link_map, objects = build_qjs(source_root, args.work_dir,
                                               args.evidence_dir, args.target)
        report["binary"] = {"sha256": sha256_file(binary), "bytes": binary.stat().st_size,
                            "machine": machine_from_binary(binary)}
        report["link_map_sha256"] = sha256_file(link_map)
        report["build_objects"] = objects
        report["native_imports"] = native_imports(binary, args.evidence_dir, args.target)
        zipapp = args.work_dir / "yt-dlp-2026.10.21.pyz"
        fetch_exact(ZIPAPP_URL, zipapp, ZIPAPP_BYTES, ZIPAPP_SHA256)
        report["zipapp_sha256"] = ZIPAPP_SHA256
        report["ejs_smoke"] = smoke_ejs(binary, zipapp, args.evidence_dir)
        # Research success is distinct from product license/notice admission.
        report["research_result"] = "PASS"
        report["native_link_notice_closure"] = "BLOCKED: collected map/imports and source notices need independent component/license review"
        return 0
    except Exception as exc:
        report["error"] = str(exc)
        return 1
    finally:
        write_report(args.evidence_dir / "result.json", report)


if __name__ == "__main__":
    sys.exit(main())
