#!/usr/bin/env python3
"""Stage six reviewed native build artifacts for a signed Whisper release."""

import argparse
import hashlib
import json
from pathlib import Path
import shutil
import tarfile


TARGETS = {
    "ubuntu-24.04": ("linux", "amd64"),
    "ubuntu-24.04-arm": ("linux", "arm64"),
    "macos-15-intel": ("darwin", "amd64"),
    "macos-15": ("darwin", "arm64"),
    "windows-2025": ("windows", "amd64"),
    "windows-11-arm": ("windows", "arm64"),
}
SOURCE_SHA256 = "ff1a9053feb509ff9d7729703355541ae9690073a6b1c40eb692c962e0dc1720"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def stage(artifacts, notices, output, source_commit, require_admitted=False):
    if len(source_commit) != 40 or any(c not in "0123456789abcdef" for c in source_commit):
        raise ValueError("source commit must be a full lowercase SHA-1")
    if not notices.is_file() or not notices.read_bytes().strip():
        raise ValueError("reviewed WHISPER-NOTICES.txt is required")
    if output.exists() and any(output.iterdir()):
        raise ValueError("release output directory must be empty")
    output.mkdir(parents=True, exist_ok=True)
    rows = []
    inventory = []
    evidence_files = []
    for label, (os_name, arch) in TARGETS.items():
        folder = artifacts / f"s503-native-build-{label}"
        report_path = folder / "build-report.json"
        runner_path = folder / "runner-identity.json"
        report = json.loads(report_path.read_text())
        runner = json.loads(runner_path.read_text())
        evidence_files.extend((report_path, runner_path))
        platform = f"{os_name}/{arch}"
        if report.get("platform") != platform or report.get("runner_label") != label:
            raise ValueError(f"{label}: build report target mismatch")
        expected_os = {"linux": "Linux", "darwin": "macOS", "windows": "Windows"}[os_name]
        if (runner.get("runner_label") != label or runner.get("actual_arch") != arch
                or runner.get("actual_os") != expected_os or runner.get("translated") == "1"):
            raise ValueError(f"{label}: runner identity mismatch")
        if runner.get("commit_sha") != source_commit:
            raise ValueError(f"{label}: source commit mismatch")
        if report.get("whisper_version") != "1.9.5" or report.get("source_archive_sha256") != SOURCE_SHA256:
            raise ValueError(f"{label}: upstream source pin mismatch")
        if report.get("status") != "built_and_cached" or not report.get("cpu_only") or not report.get("portable_cpu") or not report.get("openmp_disabled") or not report.get("gpl_free"):
            raise ValueError(f"{label}: build policy mismatch")
        if not isinstance(report.get("product_admitted"), bool):
            raise ValueError(f"{label}: product admission field is missing")
        if require_admitted and report.get("product_admitted") is not True:
            raise ValueError(f"{label}: product admission is still open")
        binary = folder / ("whisper-cli.exe" if os_name == "windows" else "whisper-cli")
        if not binary.is_file() or digest(binary) != report.get("executable_sha256") or binary.stat().st_size != report.get("executable_bytes"):
            raise ValueError(f"{label}: binary differs from build report")
        if os_name == "windows":
            link_map = folder / "whisper-cli.link.map"
            link_inventory = folder / "whisper-link-inventory.json"
            evidence = json.loads(link_inventory.read_text())
            if (not link_map.is_file() or not evidence.get("archive_members")
                    or evidence.get("binary_sha256") != digest(binary)
                    or evidence.get("map_sha256") != digest(link_map)
                    or evidence != report.get("link_map_inventory")):
                raise ValueError(f"{label}: retained linker evidence mismatch")
            evidence_files.extend((link_map, link_inventory))
        name = f"whisper-cli-{os_name}-{arch}" + (".exe" if os_name == "windows" else "")
        target = output / name
        shutil.copyfile(binary, target)
        target.chmod(0o755 if os_name != "windows" else 0o644)
        rows.append((name, digest(target)))
        inventory.append({"platform": platform, "asset": name, "sha256": digest(target),
                          "build_report_sha256": digest(report_path), "runner_identity_sha256": digest(runner_path),
                          "product_admitted": report.get("product_admitted") is True})
    notice_asset = output / "WHISPER-NOTICES.txt"
    shutil.copyfile(notices, notice_asset)
    rows.append((notice_asset.name, digest(notice_asset)))
    manifest = output / "whisper-release-manifest.json"
    manifest.write_text(json.dumps({"schema": "s503-whisper-release-v1", "source_commit": source_commit,
                                    "upstream_version": "1.9.5", "targets": inventory,
                                    "notices_sha256": digest(notice_asset)}, indent=2) + "\n")
    rows.append((manifest.name, digest(manifest)))
    evidence_archive = output / "whisper-build-evidence.tar.gz"
    with tarfile.open(evidence_archive, "w:gz") as archive:
        for file in evidence_files:
            archive.add(file, arcname=f"{file.parent.name}/{file.name}")
    rows.append((evidence_archive.name, digest(evidence_archive)))
    (output / "SHA256SUMS").write_text("".join(f"{sha}  {name}\n" for name, sha in sorted(rows)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--notices", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--require-admitted", action="store_true")
    args = parser.parse_args()
    stage(args.artifacts, args.notices, args.output, args.source_commit, args.require_admitted)


if __name__ == "__main__":
    main()
