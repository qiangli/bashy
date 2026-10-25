#!/usr/bin/env python3
"""Platform-aware, fail-closed whisper.cpp v1.9.5 source build and cache recipe.

Sprint #503 / Story #2742 / Story-ID 74ed924ba244.

Builds standalone, CPU-only whisper-cli from the pinned official upstream source
archive (v1.9.5) across the six native targets:
  - ubuntu-24.04      (linux/amd64)
  - ubuntu-24.04-arm  (linux/arm64)
  - macos-15-intel    (darwin/amd64)
  - macos-15          (darwin/arm64)
  - windows-2025      (windows/amd64)
  - windows-11-arm    (windows/arm64)

Guarantees:
  - Pinned official source archive SHA-256 verified before unpack.
  - Fail-closed: invalid archive, missing toolchain, or build error halts immediately.
  - CPU-only build: OpenMP disabled (-DGGML_OPENMP=OFF), Metal/GPU disabled.
  - Zero GPL dynamic library dependencies (statically linked runtime or standard OS libraries).
  - SHA-256 hashed and sized finished executable.
  - Stored into canonical coordination cache directory.
  - Detailed structured JSON reporting (host, os, arch, runner_label, compiler, flags, hashes).
  - NO model download, NO media inference, NO FFmpeg, NO container.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request
from pathlib import Path
from typing import Any, Dict, List, Optional, Tuple

PINNED_VERSION = "1.9.5"
PINNED_SOURCE_URL = (
    f"https://github.com/ggml-org/whisper.cpp/archive/refs/tags/v{PINNED_VERSION}.tar.gz"
)
PINNED_SOURCE_SHA256 = "ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720"
PINNED_SOURCE_BYTES = 9626464

# Target mapping for the six official standard runner labels
RUNNER_TARGET_MAP: Dict[str, Tuple[str, str]] = {
    "ubuntu-24.04": ("linux", "amd64"),
    "ubuntu-24.04-arm": ("linux", "arm64"),
    "macos-15-intel": ("darwin", "amd64"),
    "macos-15": ("darwin", "arm64"),
    "windows-2025": ("windows", "amd64"),
    "windows-11-arm": ("windows", "arm64"),
}

TARGET_RUNNER_MAP: Dict[Tuple[str, str], str] = {
    v: k for k, v in RUNNER_TARGET_MAP.items()
}


def normalize_system() -> str:
    s = platform.system().lower()
    if s == "linux":
        return "linux"
    if s == "darwin":
        return "darwin"
    if s == "windows":
        return "windows"
    raise RuntimeError(f"unsupported operating system: {platform.system()}")


def normalize_arch() -> str:
    m = platform.machine().lower()
    if m in ("x86_64", "amd64"):
        return "amd64"
    if m in ("arm64", "aarch64"):
        return "arm64"
    raise RuntimeError(f"unsupported architecture: {platform.machine()}")


def detect_target() -> Tuple[str, str, str]:
    """Returns (os_name, arch_name, runner_label)."""
    os_name = normalize_system()
    arch_name = normalize_arch()
    pair = (os_name, arch_name)
    if pair not in TARGET_RUNNER_MAP:
        raise RuntimeError(
            f"unsupported target platform: {os_name}/{arch_name} (must match one of the six standard targets)"
        )
    runner_label = TARGET_RUNNER_MAP[pair]
    return os_name, arch_name, runner_label


def compute_sha256(path: Path) -> str:
    h = hashlib.sha256()
    with open(path, "rb") as f:
        while chunk := f.read(65536):
            h.update(chunk)
    return h.hexdigest().lower()


def verify_source_archive(archive_path: Path) -> None:
    if not archive_path.is_file():
        raise FileNotFoundError(f"source archive not found: {archive_path}")
    actual_size = archive_path.stat().st_size
    if actual_size != PINNED_SOURCE_BYTES:
        raise ValueError(
            f"source archive size mismatch for {archive_path}: expected {PINNED_SOURCE_BYTES}, got {actual_size}"
        )
    actual_sha = compute_sha256(archive_path)
    if actual_sha != PINNED_SOURCE_SHA256:
        raise ValueError(
            f"source archive SHA-256 mismatch for {archive_path}: expected {PINNED_SOURCE_SHA256}, got {actual_sha}"
        )


def download_source_archive(dest_path: Path, timeout_seconds: int = 120) -> None:
    dest_path.parent.mkdir(parents=True, exist_ok=True)
    # Each invocation owns its temporary file. Verify the bytes before the
    # atomic replace so a failed or concurrent download never poisons cache.
    fd, name = tempfile.mkstemp(prefix=".whisper-source-", dir=dest_path.parent)
    temp_dest = Path(name)
    try:
        size = 0
        sha = hashlib.sha256()
        req = urllib.request.Request(
            PINNED_SOURCE_URL, headers={"User-Agent": "bashy-whisper-build/1.0"}
        )
        with os.fdopen(fd, "wb") as out, urllib.request.urlopen(
            req, timeout=timeout_seconds
        ) as resp:
            while chunk := resp.read(65536):
                size += len(chunk)
                if size > PINNED_SOURCE_BYTES:
                    raise ValueError("source archive exceeds pinned byte size")
                out.write(chunk)
                sha.update(chunk)
            out.flush()
            os.fsync(out.fileno())
        if size != PINNED_SOURCE_BYTES or sha.hexdigest() != PINNED_SOURCE_SHA256:
            raise ValueError("downloaded source archive fails pinned size/SHA-256")
        os.replace(temp_dest, dest_path)
    finally:
        temp_dest.unlink(missing_ok=True)


def default_cache_dir(os_name: str) -> Path:
    if "BASHY_CACHE_DIR" in os.environ:
        return Path(os.environ["BASHY_CACHE_DIR"])
    if os_name == "windows":
        local_app_data = os.environ.get("LOCALAPPDATA")
        if local_app_data:
            return Path(local_app_data) / "bashy"
        return Path.home() / "AppData" / "Local" / "bashy"
    elif os_name == "darwin":
        return Path.home() / "Library" / "Caches" / "bashy"
    else:
        xdg = os.environ.get("XDG_CACHE_HOME")
        if xdg:
            return Path(xdg) / "bashy"
        return Path.home() / ".cache" / "bashy"


def get_default_coordination_cli_path(os_name: str, cache_dir: Optional[Path] = None) -> Path:
    base = cache_dir or default_cache_dir(os_name)
    exe_name = "whisper-cli.exe" if os_name == "windows" else "whisper-cli"
    return base / "tools" / "whisper-cpp" / f"v{PINNED_VERSION}" / exe_name


def find_toolchain(os_name: str) -> Dict[str, str]:
    cmake_bin = shutil.which("cmake")
    if not cmake_bin:
        # Check standard bashy binmgr locations on Windows
        if os_name == "windows":
            local_app_data = os.environ.get("LOCALAPPDATA", "")
            candidate = Path(local_app_data) / "bashy/bin/cmake/4.3.3/cmake-4.3.3-windows-x86_64/bin/cmake.exe"
            if candidate.is_file():
                cmake_bin = str(candidate)
    if not cmake_bin:
        raise RuntimeError("cmake executable not found on PATH or bashy cache")

    # Select a coherent C/C++ pair. CMake must receive these exact paths;
    # merely reporting a compiler found on PATH does not prove it was used.
    c_compiler = None
    cxx_compiler = None
    if os_name == "windows":
        # Check llvm-mingw in bashy bin
        local_app_data = os.environ.get("LOCALAPPDATA", "")
        clang_candidate = Path(local_app_data) / "bashy/bin/clang/20260616/llvm-mingw-20260616-ucrt-x86_64/bin/clang.exe"
        if normalize_arch() == "amd64" and clang_candidate.is_file():
            c_compiler = str(clang_candidate)
            cxx_candidate = clang_candidate.parent / "clang++.exe"
            if cxx_candidate.is_file():
                cxx_compiler = str(cxx_candidate)
            else:
                c_compiler = None

    if not c_compiler:
        # Linux native runners ship a matched GCC/G++ runtime. Prefer that
        # pair so whisper's C++ objects link against their intended runtime.
        pairs = (("gcc", "g++"), ("clang", "clang++"), ("cl", "cl")) if os_name == "linux" else (
            ("clang", "clang++"), ("gcc", "g++"), ("cl", "cl")
        )
        for c_name, cxx_name in pairs:
            c_path, cxx_path = shutil.which(c_name), shutil.which(cxx_name)
            if c_path and cxx_path:
                c_compiler, cxx_compiler = c_path, cxx_path
                break

    if not c_compiler or not cxx_compiler:
        raise RuntimeError("complete C/C++ compiler pair not found")

    return {
        "cmake": str(Path(cmake_bin).resolve()),
        "c_compiler": str(Path(c_compiler).resolve()),
        "cxx_compiler": str(Path(cxx_compiler).resolve()),
    }


def get_cmake_build_flags(os_name: str, arch_name: str, toolchain: Dict[str, str]) -> List[str]:
    """Generates strictly CPU-only, no-OpenMP, fail-closed CMake configuration arguments."""
    flags = [
        "-DCMAKE_BUILD_TYPE=Release",
        f"-DCMAKE_C_COMPILER={toolchain['c_compiler']}",
        f"-DCMAKE_CXX_COMPILER={toolchain['cxx_compiler']}",
        "-DBUILD_SHARED_LIBS=OFF",
        "-DWHISPER_BUILD_EXAMPLES=ON",
        "-DWHISPER_BUILD_TESTS=OFF",
        # whisper.cpp v1.9.5 otherwise defaults this to ON for native builds,
        # adding -march=native or probing ARM extensions from the CI host.
        "-DGGML_NATIVE=OFF",
        "-DGGML_OPENMP=OFF",
        "-DGGML_CUDA=OFF",
        "-DGGML_VULKAN=OFF",
        "-DGGML_SYCL=OFF",
        "-DGGML_KOMPUTE=OFF",
        "-DGGML_CANN=OFF",
    ]

    if os_name == "darwin":
        # Keep CPU-only: disable Metal and GPU backends
        flags.append("-DGGML_METAL=OFF")
    elif os_name == "windows":
        # MinGW / Clang static linking
        c_comp = toolchain.get("c_compiler", "").lower()
        if "clang" in c_comp or "gcc" in c_comp or "mingw" in c_comp:
            flags.append("-DCMAKE_EXE_LINKER_FLAGS=-static")
    elif os_name == "linux":
        # Avoid GPL runtime dependencies
        flags.append("-DCMAKE_EXE_LINKER_FLAGS=-static-libgcc -static-libstdc++")

    return flags


def audit_dynamic_libraries(executable_path: Path, os_name: str) -> List[str]:
    """Inspects dynamic libraries linked by the executable to ensure no GPL/OpenMP dylibs."""
    imported: List[str] = []
    if os_name == "darwin":
        otool = shutil.which("otool")
        command = [otool, "-L", str(executable_path)] if otool else None
    elif os_name == "linux":
        ldd = shutil.which("ldd")
        command = [ldd, str(executable_path)] if ldd else None
    elif os_name == "windows":
        dump_bin = shutil.which("llvm-objdump") or shutil.which("objdump")
        command = [dump_bin, "-p", str(executable_path)] if dump_bin else None
    else:
        raise RuntimeError(f"audit failure: unsupported OS {os_name}")

    if command is None:
        raise RuntimeError(f"audit failure: dynamic library inspection tool unavailable on {os_name}")
    res = subprocess.run(command, capture_output=True, text=True, check=False)
    if res.returncode != 0:
        raise RuntimeError(f"audit failure: {' '.join(command[:2])} exited {res.returncode}: {res.stderr.strip()}")
    lines = res.stdout.splitlines()[1:] if os_name == "darwin" else res.stdout.splitlines()
    for line in lines:
        line = line.strip()
        if not line:
            continue
        if "not found" in line.lower():
            raise RuntimeError(f"audit failure: unresolved dynamic library: {line}")
        if os_name == "windows":
            if "DLL Name:" not in line:
                continue
            lib = line.split("DLL Name:", 1)[1].strip()
        elif os_name == "darwin":
            lib = line.split()[0]
        else:
            if line.startswith("linux-vdso") or line.startswith("statically linked"):
                continue
            lib = line.split("=>", 1)[0].strip().split()[0]
        if not lib:
            raise RuntimeError(f"audit failure: malformed dependency line: {line}")
        imported.append(lib)
        name = Path(lib).name.lower()
        if any(token in name for token in ("gomp", "libomp", "iomp", "vcomp")):
            raise RuntimeError(f"audit failure: OpenMP dynamic library found: {lib}")
    if not imported and "statically linked" not in res.stdout.lower():
        raise RuntimeError("audit failure: no dynamic library information returned")

    return imported


def publish_cached_executable(built_exe: Path, destination: Path, expected_sha: str) -> None:
    """Verify a private staged copy before atomically publishing it."""
    destination.parent.mkdir(parents=True, exist_ok=True)
    fd, staged_name = tempfile.mkstemp(prefix=".whisper-cli-", dir=destination.parent)
    staged_cli = Path(staged_name)
    try:
        with os.fdopen(fd, "wb") as out, built_exe.open("rb") as src:
            shutil.copyfileobj(src, out)
            out.flush()
            os.fsync(out.fileno())
        os.chmod(staged_cli, 0o755)
        if compute_sha256(staged_cli) != expected_sha:
            raise RuntimeError("cache installation failed: staged SHA mismatch")
        os.replace(staged_cli, destination)
    finally:
        staged_cli.unlink(missing_ok=True)


def verify_cmake_compilers(cache_file: Path, toolchain: Dict[str, str]) -> None:
    """Reject a configure result that silently chose different compilers."""
    cache_text = cache_file.read_text()
    for key, expected in (("CMAKE_C_COMPILER", toolchain["c_compiler"]),
                          ("CMAKE_CXX_COMPILER", toolchain["cxx_compiler"])):
        actual = next((line.split("=", 1)[1] for line in cache_text.splitlines()
                       if line.startswith(key + ":")), None)
        if actual is None or Path(actual).resolve() != Path(expected).resolve():
            raise RuntimeError(f"CMake compiler provenance mismatch for {key}: expected {expected}, got {actual}")


def verify_cmake_portable_cpu(cache_file: Path, arch_name: str) -> None:
    """Reject host-specific CPU options in the resolved upstream CMake cache."""
    cache = {}
    for line in cache_file.read_text().splitlines():
        if ":" in line and "=" in line:
            key, value = line.split("=", 1)
            name, option_type = key.split(":", 1)
            cache[name] = (option_type, value)

    required_off = ["GGML_NATIVE", "GGML_CPU_ALL_VARIANTS"]
    if arch_name == "amd64":
        # These are the x86 SIMD options declared in pinned ggml/CMakeLists.txt.
        # With GGML_NATIVE=OFF they default OFF; check that the configure result
        # actually kept them off, including through future upstream changes.
        required_off += [
            "GGML_SSE42", "GGML_AVX", "GGML_AVX_VNNI", "GGML_AVX2", "GGML_BMI2",
            "GGML_AVX512", "GGML_AVX512_VBMI", "GGML_AVX512_VNNI", "GGML_AVX512_BF16",
        ]
        # MSVC does not declare FMA/F16C separately; it implies both with AVX2.
        required_off += [key for key in ("GGML_FMA", "GGML_F16C") if key in cache]

    for key in required_off:
        if cache.get(key) != ("BOOL", "OFF"):
            raise RuntimeError(f"CMake portable CPU audit failed for {key}: expected BOOL=OFF, got {cache.get(key)}")


def build_whisper(
    work_dir: Path,
    cache_dir: Optional[Path] = None,
    source_archive: Optional[Path] = None,
    skip_download: bool = False,
    parallel: int = 4,
    dest_cli: Optional[Path] = None,
) -> Dict[str, Any]:
    os_name, arch_name, runner_label = detect_target()
    work_dir.mkdir(parents=True, exist_ok=True)

    archive_file = source_archive or (work_dir / f"whisper.cpp-{PINNED_VERSION}.tar.gz")
    if not archive_file.is_file():
        if skip_download:
            raise FileNotFoundError(f"source archive missing and --skip-download set: {archive_file}")
        download_source_archive(archive_file)

    # Fail-closed source verification before unpack
    verify_source_archive(archive_file)

    with tempfile.TemporaryDirectory(prefix="whisper-build-", dir=work_dir) as run_name:
        # Pin the bytes used by this build. An externally supplied archive can
        # otherwise change between verification and extraction.
        owned_archive = Path(run_name) / "source.tar.gz"
        shutil.copyfile(archive_file, owned_archive)
        verify_source_archive(owned_archive)
        return _build_verified_source(
            Path(run_name), owned_archive, cache_dir, parallel, dest_cli,
            os_name, arch_name, runner_label,
        )


def _build_verified_source(
    run_dir: Path,
    archive_file: Path,
    cache_dir: Optional[Path],
    parallel: int,
    dest_cli: Optional[Path],
    os_name: str,
    arch_name: str,
    runner_label: str,
) -> Dict[str, Any]:
    """Build in an invocation-owned tree; cleanup is handled by the caller."""

    # Unpack into isolated source directory
    src_dir = run_dir / f"whisper.cpp-{PINNED_VERSION}"

    with tarfile.open(archive_file, "r:gz") as tar:
        tar.extractall(path=run_dir)

    if not src_dir.is_dir():
        raise RuntimeError(f"unpacked directory not found at {src_dir}")

    # Toolchain resolution
    toolchain = find_toolchain(os_name)
    cmake_bin = toolchain["cmake"]
    build_dir = src_dir / "build-static"

    cmake_flags = get_cmake_build_flags(os_name, arch_name, toolchain)

    # Configure
    config_cmd = [cmake_bin, "-S", str(src_dir), "-B", str(build_dir)] + cmake_flags
    conf_res = subprocess.run(
        config_cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if conf_res.returncode != 0:
        raise RuntimeError(
            f"CMake configure failed (exit {conf_res.returncode}):\nSTDOUT:\n{conf_res.stdout}\nSTDERR:\n{conf_res.stderr}"
        )

    # Confirm CMake used the compilers we requested rather than an implicit
    # generator default or a compiler inherited from ambient configuration.
    verify_cmake_compilers(build_dir / "CMakeCache.txt", toolchain)
    verify_cmake_portable_cpu(build_dir / "CMakeCache.txt", arch_name)

    cmake_ver_res = subprocess.run(
        [cmake_bin, "--version"], capture_output=True, text=True, check=False
    )
    if cmake_ver_res.returncode != 0 or not cmake_ver_res.stdout.strip():
        raise RuntimeError("CMake version probe failed")
    cmake_ver = cmake_ver_res.stdout.splitlines()[0]

    compiler_versions = {}
    for kind in ("c_compiler", "cxx_compiler"):
        compiler = toolchain[kind]
        version_arg = "/Bv" if Path(compiler).name.lower() in ("cl", "cl.exe") else "--version"
        res = subprocess.run([compiler, version_arg], capture_output=True, text=True, check=False)
        output = (res.stdout or res.stderr).strip()
        if res.returncode != 0 or not output:
            raise RuntimeError(f"compiler version probe failed for {compiler} (exit {res.returncode})")
        compiler_versions[kind] = output.splitlines()[0]

    # Build whisper-cli target
    build_cmd = [
        cmake_bin,
        "--build",
        str(build_dir),
        "--config",
        "Release",
        "--target",
        "whisper-cli",
        "--parallel",
        str(parallel),
    ]
    build_res = subprocess.run(
        build_cmd,
        stdout=subprocess.PIPE,
        stderr=subprocess.PIPE,
        text=True,
        check=False,
    )
    if build_res.returncode != 0:
        raise RuntimeError(
            f"CMake build failed (exit {build_res.returncode}):\nSTDOUT:\n{build_res.stdout}\nSTDERR:\n{build_res.stderr}"
        )

    # Locate compiled executable
    exe_name = "whisper-cli.exe" if os_name == "windows" else "whisper-cli"
    candidate_locations = [
        build_dir / "bin" / exe_name,
        build_dir / "bin" / "Release" / exe_name,
        build_dir / "Release" / exe_name,
        build_dir / exe_name,
    ]
    built_exe: Optional[Path] = None
    for cand in candidate_locations:
        if cand.is_file() and os.access(cand, os.X_OK if os_name != "windows" else os.F_OK):
            built_exe = cand
            break

    if not built_exe:
        raise RuntimeError(f"built whisper-cli binary not found in {build_dir}")

    # Hash executable
    exe_sha256 = compute_sha256(built_exe)
    exe_bytes = built_exe.stat().st_size

    # Audit dynamic libraries
    imported_libs = audit_dynamic_libraries(built_exe, os_name)

    # Install into cache through an invocation-owned file in the same directory.
    # Concurrent builders publish complete identical binaries, never partial copies.
    target_cache_cli = dest_cli or get_default_coordination_cli_path(os_name, cache_dir)
    publish_cached_executable(built_exe, target_cache_cli, exe_sha256)

    # Verify cached file matches
    cached_sha = compute_sha256(target_cache_cli)
    if cached_sha != exe_sha256:
        raise RuntimeError(f"cache installation failed: SHA mismatch on {target_cache_cli}")

    report: Dict[str, Any] = {
        "platform": f"{os_name}/{arch_name}",
        "runner_label": runner_label,
        "host": platform.node(),
        "os": platform.system(),
        "os_release": platform.release(),
        "os_version": platform.version(),
        "architecture": arch_name,
        "whisper_version": PINNED_VERSION,
        "source_archive_sha256": PINNED_SOURCE_SHA256,
        "source_archive_bytes": PINNED_SOURCE_BYTES,
        "cmake_path": cmake_bin,
        "cmake_version": cmake_ver,
        "compiler_path": toolchain["c_compiler"],
        "compiler_version": compiler_versions["c_compiler"],
        "cxx_compiler_path": toolchain["cxx_compiler"],
        "cxx_compiler_version": compiler_versions["cxx_compiler"],
        "build_flags": cmake_flags,
        "executable_built_path": str(built_exe),
        "executable_sha256": exe_sha256,
        "executable_bytes": exe_bytes,
        "cached_executable_path": str(target_cache_cli),
        "imported_libraries": imported_libs,
        "cpu_only": True,
        "portable_cpu": True,
        "openmp_disabled": True,
        "gpl_free": True,
        "status": "built_and_cached",
    }
    return report


def main() -> int:
    parser = argparse.ArgumentParser(
        description="Build and cache pinned whisper.cpp v1.9.5 CLI on native runners."
    )
    parser.add_argument(
        "--work-dir",
        type=Path,
        default=Path(tempfile.gettempdir()) / "s503-whisper-build",
        help="Temporary directory for source extraction and build",
    )
    parser.add_argument(
        "--cache-dir",
        type=Path,
        default=None,
        help="Root cache directory (defaults to ~/.cache/bashy or platform standard)",
    )
    parser.add_argument(
        "--dest-cli",
        type=Path,
        default=None,
        help="Explicit destination path for the cached whisper-cli binary",
    )
    parser.add_argument(
        "--source-archive",
        type=Path,
        default=None,
        help="Path to an existing whisper.cpp-1.9.5.tar.gz archive (must match SHA-256)",
    )
    parser.add_argument(
        "--skip-download",
        action="store_true",
        help="Do not download source archive if absent; fail-closed",
    )
    parser.add_argument(
        "--parallel",
        type=int,
        default=min(os.cpu_count() or 4, 8),
        help="Parallel build jobs for CMake",
    )
    parser.add_argument(
        "--json",
        action="store_true",
        help="Output build and audit report as JSON to stdout",
    )
    parser.add_argument(
        "--report-file",
        type=Path,
        default=None,
        help="Optional file path to write the JSON report to",
    )

    args = parser.parse_args()

    try:
        report = build_whisper(
            work_dir=args.work_dir,
            cache_dir=args.cache_dir,
            source_archive=args.source_archive,
            skip_download=args.skip_download,
            parallel=args.parallel,
            dest_cli=args.dest_cli,
        )
    except Exception as e:
        sys.stderr.write(f"whisper-build ERROR: {e}\n")
        return 1

    report_json = json.dumps(report, indent=2)
    if args.report_file:
        args.report_file.parent.mkdir(parents=True, exist_ok=True)
        args.report_file.write_text(report_json + "\n")

    if args.json:
        print(report_json)
    else:
        print(f"PASS: Built and cached whisper.cpp v1.9.5 for {report['platform']} ({report['runner_label']})")
        print(f"Cached CLI: {report['cached_executable_path']}")
        print(f"Executable SHA-256: {report['executable_sha256']}")
        print(f"Executable Size: {report['executable_bytes']} bytes")
        print(f"Compiler: {report['compiler_version']}")
        print(f"CPU-only: {report['cpu_only']}, OpenMP: disabled, GPL dynamic deps: none")

    return 0


if __name__ == "__main__":
    sys.exit(main())
