#!/usr/bin/env python3
"""Hosted Linux experiment: compare a shared GCC runtime link to S503's static candidate.

This does not install the CLI, run inference, or admit a product dependency.
The existing pinned whisper.cpp source and portable CPU configuration are reused;
only the final GCC runtime link policy changes. Sprint #503 / Story #2742.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import os
import platform
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path

import build_whisper as pinned


RUNTIME_ARCHIVES = ("libstdc++", "libsupc++", "libgcc")
GLIBC_STARTUP_NAMES = ("Scrt1.o", "crti.o", "crtn.o")
NEEDED = re.compile(r"\(NEEDED\).*Shared library: \[([^\]]+)\]")
VERSION_NAME = re.compile(r"\bName: ((?:GLIBCXX|GCC|GLIBC|CXXABI)_[A-Za-z0-9_.]+)")
LDD_PROVIDER = re.compile(r"^\s*(lib(?:stdc\+\+|gcc_s)\.so\.[0-9]+)\s+=>\s+(/\S+)", re.M)


def run_checked(command: list[str], *, output_file: Path | None = None,
                timeout: int = 120) -> str:
    result = subprocess.run(command, capture_output=True, text=True, check=False,
                            timeout=timeout)
    output = result.stdout + result.stderr
    if output_file is not None:
        output_file.write_text(output)
    if result.returncode != 0:
        raise RuntimeError(f"{command[0]} exited {result.returncode}; see {output_file or output[-2000:]}")
    return result.stdout


def source_tree(archive: Path, destination: Path) -> Path:
    pinned.verify_source_archive(archive)
    with tarfile.open(archive, "r:gz") as source:
        source.extractall(destination, filter="data")
    result = destination / f"whisper.cpp-{pinned.PINNED_VERSION}"
    if not result.is_dir():
        raise RuntimeError("pinned source archive did not yield the expected directory")
    return result


def package_receipt(path: Path) -> dict[str, str]:
    """Bind an observed runtime file to the installed Ubuntu package bytes."""
    resolved = path.resolve(strict=True)
    owners = run_checked(["dpkg-query", "-S", str(resolved)]).strip()
    if owners.count("\n") or owners.count(": ") != 1:
        raise RuntimeError(f"package owner missing for {resolved}")
    package, owned_path = owners.split(": ", 1)
    if "," in package or owned_path != str(resolved):
        raise RuntimeError(f"ambiguous package owner for {resolved}: {owners}")
    version = run_checked(["dpkg-query", "-W", "-f=${Version}", package]).strip()
    if not version:
        raise RuntimeError(f"package version missing for {resolved}")
    return {"path": str(resolved), "sha256": pinned.compute_sha256(resolved),
            "package": package, "package_version": version}


def dpkg_source_receipt(package: str) -> dict[str, str]:
    fields = run_checked(["dpkg-query", "-W",
                          "-f=${binary:Package}\t${Version}\t${source:Package}\t${source:Version}\t${db:Status-Status}",
                          package]).strip().split("\t")
    if (len(fields) != 5 or fields[0] != package or not fields[1] or
            fields[2] != "glibc" or not fields[3] or fields[4] != "installed"):
        raise RuntimeError(f"unexpected glibc package/source metadata: {fields}")
    return {"package": fields[0], "package_version": fields[1],
            "source_package": fields[2], "source_package_version": fields[3]}


def glibc_startup_receipts(direct_objects: list[str], arch: str,
                           root: Path = Path("/")) -> dict:
    """Bind the three direct glibc inputs to installed files and source metadata.

    This is provenance, not a file-level LGPL ruling or a product admission.
    """
    root = root.resolve(strict=True)
    triplet = {"amd64": "x86_64-linux-gnu", "arm64": "aarch64-linux-gnu"}[arch]
    dpkg_arch = {"amd64": "amd64", "arm64": "arm64"}[arch]
    expected_dir = root / "usr/lib" / triplet
    objects = {}
    for raw in direct_objects:
        name = Path(raw).name
        if name not in GLIBC_STARTUP_NAMES:
            continue
        if name in objects:
            raise RuntimeError(f"duplicate glibc startup object: {name}")
        resolved = Path(raw).resolve(strict=True)
        if resolved != expected_dir / name:
            raise RuntimeError(f"unexpected glibc startup path: {raw} -> {resolved}")
        receipt = package_receipt(resolved)
        if receipt["package"] != f"libc6-dev:{dpkg_arch}":
            raise RuntimeError(f"unexpected glibc startup owner for {name}: {receipt['package']}")
        objects[name] = receipt
    if set(objects) != set(GLIBC_STARTUP_NAMES):
        raise RuntimeError(f"glibc startup objects incomplete: {sorted(objects)}")
    versions = {receipt["package_version"] for receipt in objects.values()}
    if len(versions) != 1:
        raise RuntimeError(f"glibc startup package versions disagree: {sorted(versions)}")

    package = f"libc6-dev:{dpkg_arch}"
    source = dpkg_source_receipt(package)
    if source["package_version"] not in versions:
        raise RuntimeError("glibc object package version differs from source receipt")

    copyright_path = (root / "usr/share/doc/libc6-dev/copyright").resolve(strict=True)
    if not copyright_path.is_file() or not 0 < copyright_path.stat().st_size <= 2 << 20:
        raise RuntimeError(f"glibc package copyright missing or oversized: {copyright_path}")
    copyright_text = copyright_path.read_text(encoding="utf-8")
    if "GNU C Library" not in copyright_text or "Lesser General Public License" not in copyright_text:
        raise RuntimeError("glibc package copyright lacks expected library/license markers")
    copyright_receipt = package_receipt(copyright_path)
    copyright_source = dpkg_source_receipt(copyright_receipt["package"])
    if (copyright_receipt["package_version"] != source["package_version"] or
            copyright_source["source_package_version"] != source["source_package_version"]):
        raise RuntimeError("glibc copyright package version differs from object package")
    return {
        "schema": "s503-glibc-startup-provenance-v1",
        "platform": f"linux/{arch}",
        "binary_package": package,
        "binary_package_version": source["package_version"],
        "source_package": source["source_package"],
        "source_package_version": source["source_package_version"],
        "copyright": {**copyright_receipt,
                      "source_package": copyright_source["source_package"],
                      "source_package_version": copyright_source["source_package_version"],
                      "bytes": copyright_path.stat().st_size},
        "objects": objects,
        "license_review_status": "open; file-level LGPL and linked-byte review required",
        "product_admitted": False,
    }


def main() -> int:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--work-dir", required=True, type=Path)
    parser.add_argument("--output-dir", required=True, type=Path)
    parser.add_argument("--static-reference", required=True, type=Path)
    parser.add_argument("--expected-arch", required=True, choices=("amd64", "arm64"))
    parser.add_argument("--parallel", type=int, default=4)
    args = parser.parse_args()
    if args.parallel < 1 or args.parallel > 8:
        parser.error("--parallel must be between 1 and 8")

    os_name, arch, runner_label = pinned.detect_target()
    if os_name != "linux" or arch != args.expected_arch:
        raise RuntimeError(f"expected native linux/{args.expected_arch}, found {os_name}/{arch}")
    reference = json.loads(args.static_reference.read_text())
    baseline = reference["targets"][f"linux/{arch}"]
    if (reference["source_archive_sha256"] != pinned.PINNED_SOURCE_SHA256 or
            reference["whisper_version"] != pinned.PINNED_VERSION or
            baseline["runner_label"] != runner_label):
        raise RuntimeError("static reference does not match the pinned source or native runner")

    output = args.output_dir.resolve()
    output.mkdir(parents=True, exist_ok=True)
    work = args.work_dir.resolve()
    work.mkdir(parents=True, exist_ok=True)
    archive = work / f"whisper.cpp-{pinned.PINNED_VERSION}.tar.gz"
    if not archive.is_file():
        pinned.download_source_archive(archive)
    pinned.verify_source_archive(archive)

    with tempfile.TemporaryDirectory(prefix="s503-shared-runtime-", dir=work) as temporary:
        root = Path(temporary)
        copied_archive = root / "source.tar.gz"
        shutil.copyfile(archive, copied_archive)
        source = source_tree(copied_archive, root)
        build = root / "build-shared-runtime"
        linker_map = root / "whisper-cli.link.map"
        toolchain = pinned.find_toolchain("linux")
        flags = pinned.get_cmake_build_flags("linux", arch, toolchain, linker_map)
        static_link = [flag for flag in flags if flag.startswith("-DCMAKE_EXE_LINKER_FLAGS=")]
        if len(static_link) != 1 or "-static-libgcc -static-libstdc++" not in static_link[0]:
            raise RuntimeError("pinned Linux link policy changed; review this experiment")
        flags.remove(static_link[0])
        flags.extend(("-DCMAKE_EXPORT_COMPILE_COMMANDS=ON",
                      f"-DCMAKE_EXE_LINKER_FLAGS=-shared-libgcc -Wl,-Map={linker_map}"))
        if arch == "arm64":
            # GCC's default outlined AArch64 atomics pull LSE helper members
            # from libgcc.a even with -shared-libgcc. Baseline LL/SC atomics
            # avoid those static helpers and remain portable on ARMv8 hosts.
            flags.extend(("-DCMAKE_C_FLAGS=-mno-outline-atomics",
                          "-DCMAKE_CXX_FLAGS=-mno-outline-atomics"))
        if any("-static-libgcc" in flag or "-static-libstdc++" in flag for flag in flags):
            raise RuntimeError("static GCC runtime flag remains in shared experiment")

        run_checked([toolchain["cmake"], "-S", str(source), "-B", str(build), *flags],
                    output_file=output / "configure.log", timeout=300)
        pinned.verify_cmake_compilers(build / "CMakeCache.txt", toolchain)
        pinned.verify_cmake_portable_cpu(build / "CMakeCache.txt", arch)
        compile_commands = build / "compile_commands.json"
        commands = json.loads(compile_commands.read_text())
        if not isinstance(commands, list) or not commands:
            raise RuntimeError("CMake produced no compile commands")
        shutil.copyfile(compile_commands, output / "compile_commands.json")

        build_stdout = run_checked(
            [toolchain["cmake"], "--build", str(build), "--config", "Release",
             "--target", "whisper-cli", "--parallel", str(args.parallel), "--verbose"],
            output_file=output / "build.log", timeout=2400)
        if (output / "build.log").stat().st_size > 16 << 20:
            raise RuntimeError("build log exceeds 16 MiB evidence limit")
        binary = build / "bin" / "whisper-cli"
        if not binary.is_file():
            raise RuntimeError("whisper-cli was not built")
        shutil.copyfile(binary, output / "whisper-cli")
        shutil.copyfile(linker_map, output / "whisper-cli.link.map")
        inventory = pinned.inspect_linux_link_map(linker_map, binary, build_stdout)
        (output / "link-inventory.json").write_text(json.dumps(inventory, indent=2) + "\n")
        (output / "final-link-command.txt").write_text(inventory["final_link_command"] + "\n")

        readelf = shutil.which("readelf")
        if not readelf:
            raise RuntimeError("readelf is required on the hosted Linux runner")
        dynamic = run_checked([readelf, "-d", str(binary)],
                              output_file=output / "elf-dynamic.txt")
        versions = run_checked([readelf, "--version-info", str(binary)],
                               output_file=output / "elf-versions.txt")
        needed = sorted(set(NEEDED.findall(dynamic)))
        ldd_output = run_checked(["ldd", str(binary)], output_file=output / "ldd.txt")
        providers = {name: package_receipt(Path(path))
                     for name, path in LDD_PROVIDER.findall(ldd_output)}
        if set(providers) != {"libstdc++.so.6", "libgcc_s.so.1"}:
            raise RuntimeError("host GCC runtime providers could not be identified")
        required_versions = {
            prefix: sorted({name for name in VERSION_NAME.findall(versions)
                            if name.startswith(prefix + "_")})
            for prefix in ("GLIBCXX", "GCC", "GLIBC", "CXXABI")
        }
        retained_runtime = [
            {"archive": row["archive"], "member": row["member"]}
            for row in inventory["runtime_archive_members"]
            if Path(row["archive"]).name.lower().startswith(RUNTIME_ARCHIVES)
        ]
        startup_objects = {
            Path(obj).name: package_receipt(Path(obj))
            for obj in inventory["direct_objects"]
            if Path(obj).name in ("crtbeginS.o", "crtendS.o")
        }
        if set(startup_objects) != {"crtbeginS.o", "crtendS.o"}:
            raise RuntimeError("direct GCC startup objects missing from map")
        glibc_startup = glibc_startup_receipts(inventory["direct_objects"], arch)
        glibc_startup.update({
            "candidate_commit": os.environ.get("GITHUB_SHA", ""),
            "runner_label": runner_label,
            "binary_sha256": pinned.compute_sha256(output / "whisper-cli"),
            "link_map_sha256": pinned.compute_sha256(output / "whisper-cli.link.map"),
        })
        copyright_copy = output / "glibc-package-copyright"
        shutil.copyfile(glibc_startup["copyright"]["path"], copyright_copy)
        if pinned.compute_sha256(copyright_copy) != glibc_startup["copyright"]["sha256"]:
            raise RuntimeError("retained glibc copyright bytes changed during capture")
        glibc_startup["copyright"]["artifact"] = copyright_copy.name
        glibc_startup_path = output / "glibc-startup-provenance.json"
        glibc_startup_path.write_text(json.dumps(glibc_startup, indent=2) + "\n")
        # The compiler driver can still add direct GCC startup objects. The
        # claim here is limited to archive members; retain direct objects too.
        compiler_versions = {
            key: run_checked([toolchain[key], "--version"]).splitlines()[0]
            for key in ("c_compiler", "cxx_compiler")
        }
        passed = (not retained_runtime and "libstdc++.so.6" in needed and
                  bool(required_versions["GLIBCXX"]) and bool(required_versions["GLIBC"]))
        report = {
            "schema": "s503-linux-shared-gcc-probe-v1",
            "platform": f"linux/{arch}",
            "runner_label": runner_label,
            "host_machine": platform.uname().machine,
            "commit_sha": os.environ.get("GITHUB_SHA", ""),
            "source_archive_sha256": pinned.PINNED_SOURCE_SHA256,
            "whisper_version": pinned.PINNED_VERSION,
            "compiler_paths": toolchain,
            "compiler_versions": compiler_versions,
            "cmake_flags": flags,
            "compile_commands_count": len(commands),
            "compile_commands_sha256": pinned.compute_sha256(output / "compile_commands.json"),
            "binary_sha256": pinned.compute_sha256(output / "whisper-cli"),
            "binary_bytes": (output / "whisper-cli").stat().st_size,
            "map_sha256": pinned.compute_sha256(output / "whisper-cli.link.map"),
            "dynamic_needed": needed,
            "host_gcc_runtime_providers": providers,
            "gcc_startup_objects": startup_objects,
            "glibc_startup_objects": glibc_startup,
            "glibc_startup_provenance_sha256": pinned.compute_sha256(glibc_startup_path),
            "version_requirements": required_versions,
            "gcc_archive_members": retained_runtime,
            "gcc_direct_objects": [obj for obj in inventory["direct_objects"]
                                   if "/gcc/" in obj],
            "static_reference": baseline,
            "comparison": {
                "same_c_compiler_version": compiler_versions["c_compiler"] == baseline["compiler_version"],
                "static_binary_bytes": baseline["executable_bytes"],
                "shared_binary_bytes": (output / "whisper-cli").stat().st_size,
                "static_gcc_archive_member_count": sum(
                    row["count"] for row in baseline["runtime_archive_members"]),
                "shared_gcc_archive_member_count": len(retained_runtime),
                "static_ldd_imports": baseline["imported_libraries"],
                "shared_dt_needed": needed,
            },
            "probe_passed": passed,
            "product_admitted": False,
            "license_policy_status": "open; host-provided GPL runtime requires policy review",
        }
        (output / "probe-report.json").write_text(json.dumps(report, indent=2) + "\n")
        if not passed:
            raise RuntimeError("shared-runtime probe failed; inspect retained map and ELF evidence")
        print(json.dumps({"platform": report["platform"], "probe_passed": passed,
                          "dynamic_needed": needed, "gcc_archive_members": 0,
                          "product_admitted": False}, indent=2))
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except Exception as exc:
        print(f"s503 shared GCC runtime probe ERROR: {exc}", file=sys.stderr)
        sys.exit(1)
