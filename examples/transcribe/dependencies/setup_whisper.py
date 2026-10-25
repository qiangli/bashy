#!/usr/bin/env python3
"""Explicit operator source-build setup while whisper-cli has no release pin.

This command is never called by transcription, adapter registration or Bashy
startup. It uses the verified v1.9.5 recipe and records the installed binary's
exact hash for the seed adapter's existing path/hash runtime check.
"""

from __future__ import annotations

import argparse
import json
import os
from pathlib import Path
import sys
import tempfile
from typing import Any

import build_whisper


RECEIPT_NAME = "source-build-receipt.json"


def _atomic_json(path: Path, value: dict[str, Any]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, staged_name = tempfile.mkstemp(prefix=".whisper-setup-", dir=path.parent)
    staged = Path(staged_name)
    try:
        with os.fdopen(fd, "w", encoding="utf-8") as out:
            json.dump(value, out, indent=2, sort_keys=True)
            out.write("\n")
            out.flush()
            os.fsync(out.fileno())
        os.replace(staged, path)
    finally:
        staged.unlink(missing_ok=True)


def record_source_build(report: dict[str, Any], cache_dir: Path | None = None) -> dict[str, Any]:
    """Validate a recipe result and atomically publish its runtime coordinates."""
    os_name, arch_name, runner_label = build_whisper.detect_target()
    expected_cli = build_whisper.get_default_coordination_cli_path(os_name, cache_dir).resolve()
    flags = report.get("build_flags")
    if any((
        report.get("status") != "built_and_cached",
        report.get("platform") != f"{os_name}/{arch_name}",
        report.get("runner_label") != runner_label,
        report.get("whisper_version") != build_whisper.PINNED_VERSION,
        report.get("source_archive_sha256") != build_whisper.PINNED_SOURCE_SHA256,
        report.get("source_archive_bytes") != build_whisper.PINNED_SOURCE_BYTES,
        report.get("cpu_only") is not True,
        report.get("portable_cpu") is not True,
        report.get("openmp_disabled") is not True,
        report.get("gpl_free") is not True,
        not isinstance(flags, list) or "-DGGML_NATIVE=OFF" not in flags,
    )):
        raise RuntimeError("source-build report does not meet the reviewed v1.9.5 CPU recipe")
    reported_path = report.get("cached_executable_path")
    if not isinstance(reported_path, str) or not reported_path:
        raise RuntimeError("source-build report lacks a cached executable path")
    reported_cli = Path(reported_path).resolve(strict=True)
    if reported_cli != expected_cli or not reported_cli.is_file():
        raise RuntimeError("source-build executable is outside the canonical binmgr coordination cache")
    digest = report.get("executable_sha256")
    if not isinstance(digest, str) or len(digest) != 64 or any(ch not in "0123456789abcdef" for ch in digest):
        raise RuntimeError("source-build report lacks an exact executable SHA-256")
    size = report.get("executable_bytes")
    if type(size) is not int or size <= 0 or reported_cli.stat().st_size != size:
        raise RuntimeError("source-build executable byte count differs from the report")
    if build_whisper.compute_sha256(reported_cli) != digest:
        raise RuntimeError("source-build executable SHA-256 differs from the report")

    receipt_path = reported_cli.parent / RECEIPT_NAME
    receipt = {
        "schema": "s503-whisper-source-setup-v1",
        "install_method": "explicit_pinned_source_build",
        "platform": report["platform"],
        "runner_label": runner_label,
        "whisper_version": build_whisper.PINNED_VERSION,
        "source_archive_sha256": build_whisper.PINNED_SOURCE_SHA256,
        "executable_path": str(reported_cli),
        "executable_sha256": digest,
        "executable_bytes": size,
        "registration": {"WHISPER_PATH": str(reported_cli), "WHISPER_SHA256": digest},
        "receipt_path": str(receipt_path),
    }
    _atomic_json(receipt_path, receipt)
    # Another setup may publish to the same cache concurrently. Runtime still
    # rehashes before every inference; detect a changed binary here as well.
    if build_whisper.compute_sha256(reported_cli) != digest:
        raise RuntimeError("source-build executable changed during receipt publication")
    return receipt


def setup(
    work_dir: Path, cache_dir: Path | None = None,
    source_archive: Path | None = None, skip_download: bool = False,
    parallel: int = 4,
) -> dict[str, Any]:
    report = build_whisper.build_whisper(
        work_dir=work_dir, cache_dir=cache_dir, source_archive=source_archive,
        skip_download=skip_download, parallel=parallel,
    )
    return record_source_build(report, cache_dir)


def main() -> int:
    parser = argparse.ArgumentParser(description="Explicitly build and record pinned whisper.cpp v1.9.5")
    parser.add_argument("--work-dir", type=Path, default=Path(tempfile.gettempdir()) / "s503-whisper-setup")
    parser.add_argument("--cache-dir", type=Path)
    parser.add_argument("--source-archive", type=Path)
    parser.add_argument("--skip-download", action="store_true")
    parser.add_argument("--parallel", type=int, default=min(os.cpu_count() or 4, 8))
    args = parser.parse_args()
    if args.parallel <= 0:
        parser.error("--parallel must be positive")
    try:
        receipt = setup(args.work_dir, args.cache_dir, args.source_archive, args.skip_download, args.parallel)
    except (OSError, ValueError, RuntimeError) as exc:
        print(f"whisper source setup rejected: {exc}", file=sys.stderr)
        return 1
    print(json.dumps(receipt, indent=2, sort_keys=True))
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
