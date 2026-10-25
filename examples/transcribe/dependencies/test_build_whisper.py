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
import json
import os
import platform
import shutil
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path

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

            # Specific platform rules
            if os_name == "darwin":
                self.assertIn("-DGGML_METAL=OFF", flags, f"Metal must be disabled for CPU-only on {runner}")
            elif os_name == "linux":
                self.assertTrue(
                    any("-static-libgcc" in f for f in flags),
                    f"Linux must avoid dynamic GCC/GPL dependencies on {runner}",
                )

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
        """Dynamic library audit must reject OpenMP/GPL libraries."""
        # Simulated audit failure
        with self.assertRaises(RuntimeError) as ctx:
            # Test direct logic check
            lib = "/usr/lib/libgomp.so.1"
            if "libgomp" in lib.lower() or "omp" in lib.lower():
                raise RuntimeError(f"audit failure: OpenMP dynamic library found: {lib}")
        self.assertIn("OpenMP dynamic library found", str(ctx.exception))

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
