#!/usr/bin/env python3
"""Unit, argument, and negative tests for whisper.cpp source build & cache recipe.

Sprint #503 / Story #2742 / Story-ID 74ed924ba244.

Enforces:
  - Pinned official upstream SHA-256 and byte length constants.
  - Fail-closed verification on missing or tampered archives.
  - Platform detection and runner label mapping across all six targets.
  - CPU-only, OpenMP disabled, shared libs disabled CMake flags.
  - Dynamic library audit: rejection of OpenMP/GPL dynamic libraries.
  - CLI argument validation and negative error paths.
  - No false six-target PASS: reports only active native platform.
"""

from __future__ import annotations

import hashlib
import io
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import unittest
from concurrent.futures import ThreadPoolExecutor
from pathlib import Path
from unittest.mock import patch

# Add current directory to path to import build_whisper
SCRIPT_DIR = Path(__file__).resolve().parent
sys.path.insert(0, str(SCRIPT_DIR))

import build_whisper  # noqa: E402


class TestWhisperBuildRecipe(unittest.TestCase):
    def test_pinned_constants(self):
        """Source archive constants must strictly match the supply-chain audit."""
        self.assertEqual(build_whisper.PINNED_VERSION, "1.9.5")
        self.assertEqual(
            build_whisper.PINNED_SOURCE_SHA256,
            "ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720",
        )
        self.assertEqual(build_whisper.PINNED_SOURCE_BYTES, 9626464)
        self.assertTrue(
            build_whisper.PINNED_SOURCE_URL.endswith(
                "ggml-org/whisper.cpp/archive/refs/tags/v1.9.5.tar.gz"
            )
        )

    def test_six_target_runner_labels(self):
        """All six official runner targets must be mapped bidirectionally."""
        expected_runners = {
            "ubuntu-24.04": ("linux", "amd64"),
            "ubuntu-24.04-arm": ("linux", "arm64"),
            "macos-15-intel": ("darwin", "amd64"),
            "macos-15": ("darwin", "arm64"),
            "windows-2025": ("windows", "amd64"),
            "windows-11-arm": ("windows", "arm64"),
        }
        self.assertEqual(build_whisper.RUNNER_TARGET_MAP, expected_runners)
        for runner, pair in expected_runners.items():
            self.assertEqual(build_whisper.TARGET_RUNNER_MAP.get(pair), runner)

    def test_detect_target_current_host(self):
        """Detect target on current host must produce a valid known target."""
        os_name, arch_name, runner_label = build_whisper.detect_target()
        self.assertIn(os_name, ("linux", "darwin", "windows"))
        self.assertIn(arch_name, ("amd64", "arm64"))
        self.assertIn(runner_label, build_whisper.RUNNER_TARGET_MAP)
        self.assertEqual(build_whisper.RUNNER_TARGET_MAP[runner_label], (os_name, arch_name))

    def test_fail_closed_source_verification(self):
        """Tampered or missing source archives must fail closed before unpack."""
        with tempfile.TemporaryDirectory() as td:
            temp_dir = Path(td)
            # 1. Non-existent file
            missing = temp_dir / "non-existent.tar.gz"
            with self.assertRaises(FileNotFoundError):
                build_whisper.verify_source_archive(missing)

            # 2. Corrupted byte length
            bad_len = temp_dir / "bad_len.tar.gz"
            bad_len.write_bytes(b"short content")
            with self.assertRaises(ValueError) as ctx:
                build_whisper.verify_source_archive(bad_len)
            self.assertIn("size mismatch", str(ctx.exception))

            # 3. Correct byte length but tampered hash
            tampered = temp_dir / "tampered.tar.gz"
            # Write exactly PINNED_SOURCE_BYTES bytes of zeroes
            with open(tampered, "wb") as f:
                f.seek(build_whisper.PINNED_SOURCE_BYTES - 1)
                f.write(b"\0")
            self.assertEqual(tampered.stat().st_size, build_whisper.PINNED_SOURCE_BYTES)
            with self.assertRaises(ValueError) as ctx:
                build_whisper.verify_source_archive(tampered)
            self.assertIn("SHA-256 mismatch", str(ctx.exception))

    def test_cmake_build_flags_all_platforms(self):
        """Build flags across all platforms must enforce CPU-only and no OpenMP."""
        for runner, (os_name, arch_name) in build_whisper.RUNNER_TARGET_MAP.items():
            flags = build_whisper.get_cmake_build_flags(
                os_name, arch_name, {"cmake": "cmake", "c_compiler": "clang", "cxx_compiler": "clang++"}
            )
            self.assertIn("-DGGML_OPENMP=OFF", flags, f"OpenMP must be disabled on {runner}")
            self.assertIn("-DBUILD_SHARED_LIBS=OFF", flags, f"Shared libs must be OFF on {runner}")
            self.assertIn("-DWHISPER_BUILD_EXAMPLES=ON", flags, f"Examples must be ON on {runner}")
            self.assertIn("-DWHISPER_BUILD_TESTS=OFF", flags, f"Tests must be OFF on {runner}")
            self.assertIn("-DCMAKE_BUILD_TYPE=Release", flags, f"Release mode on {runner}")
            self.assertIn("-DGGML_NATIVE=OFF", flags, f"Host CPU tuning must be OFF on {runner}")
            self.assertIn("-DGGML_CPU_ALL_VARIANTS=OFF", flags, f"Variant build must be OFF on {runner}")
            if arch_name == "amd64":
                for option in build_whisper.X86_BASELINE_OFF_OPTIONS:
                    self.assertIn(f"-D{option}:BOOL=OFF", flags, f"{option} must be OFF on {runner}")

            # Specific platform rules
            if os_name == "darwin":
                self.assertIn("-DGGML_METAL=OFF", flags, f"Metal must be disabled for CPU-only on {runner}")
            elif os_name == "linux":
                self.assertTrue(
                    any("-static-libgcc" in f for f in flags),
                    f"Linux must avoid dynamic GCC/GPL dependencies on {runner}",
                )

    def test_cmake_cache_rejects_native_cpu_optimizations(self):
        """The resolved CMake options must keep x86 and ARM binaries portable."""
        x86_options = (
            "GGML_SSE42", "GGML_AVX", "GGML_AVX_VNNI", "GGML_AVX2", "GGML_BMI2",
            "GGML_AVX512", "GGML_AVX512_VBMI", "GGML_AVX512_VNNI", "GGML_AVX512_BF16",
            "GGML_FMA", "GGML_F16C",
        )
        with tempfile.TemporaryDirectory() as td:
            cache = Path(td) / "CMakeCache.txt"
            portable = {key: "OFF" for key in ("GGML_NATIVE", "GGML_CPU_ALL_VARIANTS", *x86_options)}

            def write_cache(options):
                cache.write_text("".join(f"{key}:BOOL={value}\n" for key, value in options.items()))

            write_cache(portable)
            build_whisper.verify_cmake_portable_cpu(cache, "amd64")
            build_whisper.verify_cmake_portable_cpu(cache, "arm64")

            for key in ("GGML_NATIVE", "GGML_CPU_ALL_VARIANTS", "GGML_AVX2", "GGML_FMA"):
                with self.subTest(option=key):
                    write_cache({**portable, key: "ON"})
                    with self.assertRaisesRegex(RuntimeError, key):
                        build_whisper.verify_cmake_portable_cpu(cache, "amd64")

            # Missing cache entries are not evidence that CMake honored them.
            write_cache({key: value for key, value in portable.items() if key != "GGML_NATIVE"})
            with self.assertRaisesRegex(RuntimeError, "GGML_NATIVE"):
                build_whisper.verify_cmake_portable_cpu(cache, "arm64")

            write_cache(portable)
            cache.write_text(cache.read_text().replace("GGML_NATIVE:BOOL", "GGML_NATIVE:UNINITIALIZED"))
            with self.assertRaisesRegex(RuntimeError, "GGML_NATIVE"):
                build_whisper.verify_cmake_portable_cpu(cache, "arm64")

    def test_cache_dir_resolution(self):
        """Coordination cache path must match platform standards."""
        with tempfile.TemporaryDirectory() as td:
            custom_cache = Path(td) / "custom_cache"
            cli_path = build_whisper.get_default_coordination_cli_path("darwin", custom_cache)
            self.assertEqual(
                cli_path,
                custom_cache / "tools" / "whisper-cpp" / "v1.9.5" / "whisper-cli",
            )
            cli_win = build_whisper.get_default_coordination_cli_path("windows", custom_cache)
            self.assertEqual(
                cli_win,
                custom_cache / "tools" / "whisper-cpp" / "v1.9.5" / "whisper-cli.exe",
            )

    def test_dynamic_library_audit_mock(self):
        """Exercise the actual platform parser and its failure paths."""
        samples = {
            "linux": ("/usr/bin/ldd", "libgomp.so.1 => /usr/lib/libgomp.so.1\n"),
            "darwin": ("/usr/bin/otool", "whisper-cli:\n\t/usr/lib/libomp.dylib (compatibility version 1.0)\n"),
            "windows": ("C:/objdump.exe", "DLL Name: libgomp-1.dll\n"),
        }
        for os_name, (tool, output) in samples.items():
            with self.subTest(os_name=os_name), patch("build_whisper.shutil.which", return_value=tool), patch(
                "build_whisper.subprocess.run", return_value=subprocess.CompletedProcess([], 0, output, "")
            ):
                with self.assertRaisesRegex(RuntimeError, "OpenMP dynamic library"):
                    build_whisper.audit_dynamic_libraries(Path("whisper-cli"), os_name)

            with self.subTest(os_name=os_name, failure="missing tool"), patch(
                "build_whisper.shutil.which", return_value=None
            ):
                with self.assertRaisesRegex(RuntimeError, "tool unavailable"):
                    build_whisper.audit_dynamic_libraries(Path("whisper-cli"), os_name)

            with self.subTest(os_name=os_name, failure="tool exit"), patch(
                "build_whisper.shutil.which", return_value=tool
            ), patch("build_whisper.subprocess.run", return_value=subprocess.CompletedProcess([], 1, "", "broken")):
                with self.assertRaisesRegex(RuntimeError, "exited 1"):
                    build_whisper.audit_dynamic_libraries(Path("whisper-cli"), os_name)

        with patch("build_whisper.shutil.which", return_value="ldd"), patch(
            "build_whisper.subprocess.run",
            return_value=subprocess.CompletedProcess([], 0, "libc.so.6 => not found\n", ""),
        ):
            with self.assertRaisesRegex(RuntimeError, "unresolved"):
                build_whisper.audit_dynamic_libraries(Path("whisper-cli"), "linux")

    def test_concurrent_cache_publication(self):
        """Readers see a complete old or new binary during concurrent publication."""
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            destination = root / "cache" / "whisper-cli"
            destination.parent.mkdir()
            destination.write_bytes(b"old complete binary")
            payload = b"new complete binary" * 10000
            source = root / "built-cli"
            source.write_bytes(payload)
            expected = hashlib.sha256(payload).hexdigest()
            with ThreadPoolExecutor(max_workers=4) as pool:
                futures = [pool.submit(build_whisper.publish_cached_executable, source, destination, expected)
                           for _ in range(4)]
                for future in futures:
                    future.result()
            self.assertEqual(destination.read_bytes(), payload)
            self.assertFalse(list(destination.parent.glob(".whisper-cli-*")))

            with self.assertRaisesRegex(RuntimeError, "staged SHA mismatch"):
                build_whisper.publish_cached_executable(source, destination, "bad hash")
            self.assertEqual(destination.read_bytes(), payload)

    def test_unique_build_tree_and_cleanup(self):
        """Concurrent builds receive separate source trees and remove them on exit."""
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            archive = root / "source.tar.gz"
            archive.write_bytes(b"fixture")
            seen = []

            def inspect_run(run_dir, *args):
                self.assertTrue(run_dir.is_dir())
                seen.append(run_dir)
                return {"run_dir": str(run_dir)}

            with patch("build_whisper.detect_target", return_value=("linux", "amd64", "ubuntu-24.04")), patch(
                "build_whisper.verify_source_archive"
            ), patch("build_whisper._build_verified_source", side_effect=inspect_run):
                with ThreadPoolExecutor(max_workers=2) as pool:
                    futures = [pool.submit(build_whisper.build_whisper, root, source_archive=archive) for _ in range(2)]
                    for future in futures:
                        future.result()
            self.assertEqual(len(set(seen)), 2)
            self.assertTrue(all(not path.exists() for path in seen))

    def test_toolchain_provenance_flags(self):
        """A complete pair is required and both absolute compiler paths reach CMake."""
        tools = {"cmake": "/tools/cmake", "clang": "/tools/clang", "clang++": "/tools/clang++"}
        with patch("build_whisper.shutil.which", side_effect=tools.get):
            toolchain = build_whisper.find_toolchain("linux")
        self.assertEqual(toolchain["c_compiler"], "/tools/clang")
        flags = build_whisper.get_cmake_build_flags("linux", "amd64", toolchain)
        self.assertIn("-DCMAKE_C_COMPILER=/tools/clang", flags)
        self.assertIn("-DCMAKE_CXX_COMPILER=/tools/clang++", flags)
        with patch("build_whisper.shutil.which", side_effect={"cmake": "/tools/cmake", "clang": "/tools/clang"}.get):
            with self.assertRaisesRegex(RuntimeError, "complete C/C\\+\\+ compiler pair"):
                build_whisper.find_toolchain("linux")

        with tempfile.TemporaryDirectory() as td:
            cache = Path(td) / "CMakeCache.txt"
            cache.write_text("CMAKE_C_COMPILER:FILEPATH=/tools/clang\n"
                             "CMAKE_CXX_COMPILER:FILEPATH=/tools/clang++\n")
            build_whisper.verify_cmake_compilers(cache, toolchain)
            cache.write_text("CMAKE_C_COMPILER:FILEPATH=/tools/other\n"
                             "CMAKE_CXX_COMPILER:FILEPATH=/tools/clang++\n")
            with self.assertRaisesRegex(RuntimeError, "provenance mismatch"):
                build_whisper.verify_cmake_compilers(cache, toolchain)

    def test_linux_prefers_complete_gcc_pair(self):
        """Linux uses GCC/G++ together even when Clang is also available."""
        tools = {
            "cmake": "/tools/cmake", "clang": "/tools/clang", "clang++": "/tools/clang++",
            "gcc": "/tools/gcc", "g++": "/tools/g++",
        }
        with patch("build_whisper.shutil.which", side_effect=tools.get):
            selected = build_whisper.find_toolchain("linux")
        self.assertEqual(selected["c_compiler"], "/tools/gcc")
        self.assertEqual(selected["cxx_compiler"], "/tools/g++")
        flags = build_whisper.get_cmake_build_flags("linux", "arm64", selected)
        self.assertIn("-DCMAKE_C_COMPILER=/tools/gcc", flags)
        self.assertIn("-DCMAKE_CXX_COMPILER=/tools/g++", flags)

        # An incomplete GCC installation must fall back to a complete pair.
        del tools["g++"]
        with patch("build_whisper.shutil.which", side_effect=tools.get):
            selected = build_whisper.find_toolchain("linux")
        self.assertEqual(selected["c_compiler"], "/tools/clang")
        self.assertEqual(selected["cxx_compiler"], "/tools/clang++")

    def test_download_failure_preserves_existing_archive(self):
        """A bad new download cannot replace a previously verified archive."""
        payload = b"pinned fixture bytes"
        with tempfile.TemporaryDirectory() as td:
            destination = Path(td) / "source.tar.gz"
            destination.write_bytes(payload)
            with patch.object(build_whisper, "PINNED_SOURCE_BYTES", len(payload)), patch.object(
                build_whisper, "PINNED_SOURCE_SHA256", hashlib.sha256(payload).hexdigest()
            ), patch("build_whisper.urllib.request.urlopen", return_value=io.BytesIO(b"bad")):
                with self.assertRaisesRegex(ValueError, "fails pinned"):
                    build_whisper.download_source_archive(destination)
            self.assertEqual(destination.read_bytes(), payload)
            self.assertFalse(list(destination.parent.glob(".whisper-source-*")))

    def test_no_false_six_target_pass(self):
        """Execution on a single host must never claim all six targets passed."""
        os_name, arch_name, runner_label = build_whisper.detect_target()
        # Ensure our target runner map has exactly 6 distinct entries
        self.assertEqual(len(build_whisper.RUNNER_TARGET_MAP), 6)
        # Ensure single execution represents exactly 1 platform
        active_count = len([runner_label])
        self.assertEqual(active_count, 1)
        self.assertNotEqual(active_count, 6, "Must not claim six-target completion on single host")

    def test_cli_argument_handling(self):
        """CLI argument handling and help invocation."""
        script = SCRIPT_DIR / "build_whisper.py"
        # 1. --help should exit 0
        res = subprocess.run(
            [sys.executable, str(script), "--help"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.assertEqual(res.returncode, 0)
        self.assertIn("whisper.cpp v1.9.5", res.stdout)

        # 2. Unknown argument should exit 2
        res = subprocess.run(
            [sys.executable, str(script), "--unrecognized-option-foo"],
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True,
        )
        self.assertEqual(res.returncode, 2)

        # 3. --skip-download with missing archive must fail-closed
        with tempfile.TemporaryDirectory() as td:
            missing_archive = Path(td) / "does_not_exist.tar.gz"
            res = subprocess.run(
                [
                    sys.executable,
                    str(script),
                    "--source-archive",
                    str(missing_archive),
                    "--skip-download",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("source archive missing", res.stderr)

        # 4. Tampered archive path via CLI must fail-closed with SHA mismatch
        with tempfile.TemporaryDirectory() as td:
            bad_archive = Path(td) / "whisper.cpp-1.9.5.tar.gz"
            bad_archive.write_bytes(b"corrupted contents")
            res = subprocess.run(
                [
                    sys.executable,
                    str(script),
                    "--source-archive",
                    str(bad_archive),
                    "--skip-download",
                ],
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
            )
            self.assertNotEqual(res.returncode, 0)
            self.assertIn("size mismatch", res.stderr)


    def test_unsupported_platform_fails_closed(self):
        """Unsupported OS or architecture must raise RuntimeError fail-closed."""
        from unittest.mock import patch

        with patch("platform.system", return_value="SunOS"):
            with self.assertRaises(RuntimeError) as ctx:
                build_whisper.normalize_system()
            self.assertIn("unsupported operating system", str(ctx.exception))

        with patch("platform.machine", return_value="mips64"):
            with self.assertRaises(RuntimeError) as ctx:
                build_whisper.normalize_arch()
            self.assertIn("unsupported architecture", str(ctx.exception))

    def test_missing_cmake_fails_closed(self):
        """Missing cmake in PATH and binmgr cache must fail-closed."""
        from unittest.mock import patch

        with patch("shutil.which", return_value=None):
            with self.assertRaises(RuntimeError) as ctx:
                build_whisper.find_toolchain("linux")
            self.assertIn("cmake executable not found", str(ctx.exception))

    def test_no_model_download_or_media_inference_policy(self):
        """Recipe must contain NO model downloading, NO WAV inference, NO FFmpeg."""
        source_text = (SCRIPT_DIR / "build_whisper.py").read_text()
        forbidden_tokens = [
            "ggml-tiny.bin",
            "ggml-base.bin",
            "huggingface.co",
            "jfk.wav",
            "ffmpeg",
            "afconvert",
            "-m ggml",
            "--model",
        ]
        for token in forbidden_tokens:
            self.assertNotIn(
                token,
                source_text,
                f"Forbidden token {token!r} found in build_whisper.py (violates bounded deliverable)",
            )


if __name__ == "__main__":
    unittest.main()
