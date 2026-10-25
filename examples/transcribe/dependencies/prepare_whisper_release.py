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
NOTICE_PINS = Path(__file__).with_name("notices") / "source-pins.json"


def digest(path):
    return hashlib.sha256(path.read_bytes()).hexdigest()


def verified_notices(folder, platform, binary, report_path, require_admitted):
    pins = json.loads(NOTICE_PINS.read_text())
    pins_sha = digest(NOTICE_PINS)
    notice_dir = folder / "whisper-notices"
    manifest_path = notice_dir / "NOTICE_MANIFEST.json"
    bundle_path = notice_dir / "THIRD_PARTY_NOTICES.txt"
    manifest = json.loads(manifest_path.read_text())
    if (manifest.get("schema") != "s503-whisper-notice-bundle-v1"
            or manifest.get("status") != "candidate_product_not_admitted"
            or manifest.get("target") != platform
            or manifest.get("binary_sha256") != digest(binary)
            or manifest.get("binary_bytes") != binary.stat().st_size
            or manifest.get("source_archive_sha256") != SOURCE_SHA256
            or manifest.get("source_pins_sha256") != pins_sha
            or manifest.get("bundle_sha256") != digest(bundle_path)
            or manifest.get("evidence", {}).get("build_report_sha256") != digest(report_path)):
        raise ValueError(f"{platform}: notice manifest differs from binary or pinned source")
    if platform.startswith(("windows/", "linux/")):
        evidence = manifest.get("evidence", {})
        inventory_path = folder / "whisper-link-inventory.json"
        map_path = folder / "whisper-cli.link.map"
        inventory = json.loads(inventory_path.read_text())
        report = json.loads(report_path.read_text())
        if (evidence.get("inventory_sha256") != digest(inventory_path)
                or evidence.get("map_sha256") != digest(map_path)
                or inventory != report.get("link_map_inventory")
                or inventory.get("binary_sha256") != digest(binary)
                or inventory.get("map_sha256") != digest(map_path)):
            raise ValueError(f"{platform}: notice linker evidence mismatch")
    if platform.startswith("windows/"):
        arch = platform.split("/")[1]
        toolchain_path = folder / "toolchain-report.json"
        toolchain = json.loads(toolchain_path.read_text())
        asset = pins["windows_toolchain"]["assets"][arch]
        if (evidence.get("toolchain_report_sha256") != digest(toolchain_path)
                or evidence.get("toolchain_asset") != asset
                or toolchain.get("platform") != platform
                or toolchain.get("source_url") != asset["url"]
                or toolchain.get("archive_sha256") != asset["archive_sha256"]
                or toolchain.get("archive_bytes") != asset["archive_bytes"]):
            raise ValueError(f"{platform}: notice linker or toolchain evidence mismatch")
    elif platform.startswith("linux/"):
        if (inventory.get("schema") != "s503-linux-link-map-v1"
                or inventory.get("binary_name") != binary.name
                or inventory.get("map_bytes") != map_path.stat().st_size
                or not inventory.get("archive_members")
                or not inventory.get("runtime_archive_members")
                or any(row not in inventory["archive_members"] for row in inventory["runtime_archive_members"])):
            raise ValueError(f"{platform}: Linux linker inventory incomplete")
    if require_admitted:
        raise ValueError(f"{platform}: notice review remains candidate and product admission is open")
    gaps = manifest.get("open_gates")
    if not isinstance(gaps, list) or not gaps or not all(isinstance(gap, str) and gap for gap in gaps):
        raise ValueError(f"{platform}: notice review gates are missing")
    expected_gate_terms = ["Whisper and ggml objects"]
    if platform.startswith("windows/"):
        expected_gate_terms.extend(["Cephes", "source-to-prebuilt llvm-mingw"])
    elif platform.startswith("linux/"):
        expected_gate_terms.append("GPLv3 GCC Runtime Library Exception")
    else:
        expected_gate_terms.append("macOS linked system framework")
    if any(not any(term in gap for gap in gaps) for term in expected_gate_terms):
        raise ValueError(f"{platform}: mandatory notice review gate is missing")
    selected = [name for name, row in pins["notices"].items()
                if row["targets"] == "all" or (platform.startswith("windows/") and row["targets"] == "windows")]
    rows = manifest.get("notices")
    if not isinstance(rows, list) or [row.get("file") for row in rows] != [f"LICENSES/{name}" for name in selected]:
        raise ValueError(f"{platform}: notice inventory differs from pinned component set")
    sections = ["Whisper executable third-party notices\n",
                "Candidate notice bundle; product admission and license review remain open.\n"]
    files = [manifest_path, bundle_path]
    for name, row in zip(selected, rows):
        pin = pins["notices"][name]
        source = notice_dir / "LICENSES" / name
        data = source.read_bytes()
        if (digest(source) != pin["sha256"] or row.get("sha256") != pin["sha256"]
                or row.get("source_url") != pin["url"]):
            raise ValueError(f"{platform}: {name} notice source mismatch")
        sections.extend((f"\n===== {name} =====\nSource: {pin['url']}\nSHA-256: {pin['sha256']}\n\n",
                         data.decode("utf-8")))
        if not sections[-1].endswith("\n"):
            sections.append("\n")
        files.append(source)
    sections.append("\nOpen review gates:\n" + "".join(f"- {gap}\n" for gap in gaps))
    if bundle_path.read_bytes() != "".join(sections).encode("utf-8"):
        raise ValueError(f"{platform}: notice bundle text differs from pinned component set")
    return manifest_path, bundle_path, files, manifest


def stage(artifacts, output, source_commit, require_admitted=False):
    if len(source_commit) != 40 or any(c not in "0123456789abcdef" for c in source_commit):
        raise ValueError("source commit must be a full lowercase SHA-1")
    if output.exists() and any(output.iterdir()):
        raise ValueError("release output directory must be empty")
    output.mkdir(parents=True, exist_ok=True)
    rows = []
    inventory = []
    evidence_files = []
    notice_index = ["Whisper executable notice index\n",
                    "Each target notice bundle is bound to the binary digest below. Product admission remains open.\n\n"]
    notice_inventory = []
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
        if os_name in ("windows", "linux"):
            link_map = folder / "whisper-cli.link.map"
            link_inventory = folder / "whisper-link-inventory.json"
            evidence = json.loads(link_inventory.read_text())
            if (not link_map.is_file() or not evidence.get("archive_members")
                    or evidence.get("binary_sha256") != digest(binary)
                    or evidence.get("map_sha256") != digest(link_map)
                    or evidence != report.get("link_map_inventory")):
                raise ValueError(f"{label}: retained linker evidence mismatch")
            evidence_files.extend((link_map, link_inventory))
            if os_name == "windows":
                evidence_files.append(folder / "toolchain-report.json")
        notice_manifest_path, bundle_path, notice_files, notice_manifest = verified_notices(
            folder, platform, binary, report_path, require_admitted)
        evidence_files.extend(notice_files)
        name = f"whisper-cli-{os_name}-{arch}" + (".exe" if os_name == "windows" else "")
        target = output / name
        shutil.copyfile(binary, target)
        target.chmod(0o755 if os_name != "windows" else 0o644)
        rows.append((name, digest(target)))
        notice_name = f"WHISPER-NOTICES-{os_name}-{arch}.txt"
        notice_target = output / notice_name
        shutil.copyfile(bundle_path, notice_target)
        rows.append((notice_name, digest(notice_target)))
        manifest_name = f"whisper-notices-{os_name}-{arch}.json"
        manifest_target = output / manifest_name
        shutil.copyfile(notice_manifest_path, manifest_target)
        rows.append((manifest_name, digest(manifest_target)))
        notice_index.append(f"{platform}: {name} SHA-256 {digest(target)}; "
                            f"{notice_name} SHA-256 {digest(notice_target)}\n")
        notice_inventory.append({"platform": platform, "asset": notice_name,
                                 "sha256": digest(notice_target), "manifest": manifest_name,
                                 "manifest_sha256": digest(manifest_target),
                                 "open_gates": notice_manifest["open_gates"]})
        inventory.append({"platform": platform, "asset": name, "sha256": digest(target),
                          "build_report_sha256": digest(report_path), "runner_identity_sha256": digest(runner_path),
                          "product_admitted": report.get("product_admitted") is True})
    notice_asset = output / "WHISPER-NOTICES.txt"
    notice_asset.write_text("".join(notice_index))
    rows.append((notice_asset.name, digest(notice_asset)))
    manifest = output / "whisper-release-manifest.json"
    manifest.write_text(json.dumps({"schema": "s503-whisper-release-v1", "source_commit": source_commit,
                                    "upstream_version": "1.9.5", "targets": inventory,
                                    "notices_sha256": digest(notice_asset),
                                    "notice_targets": notice_inventory}, indent=2) + "\n")
    rows.append((manifest.name, digest(manifest)))
    evidence_archive = output / "whisper-build-evidence.tar.gz"
    with tarfile.open(evidence_archive, "w:gz") as archive:
        for file in evidence_files:
            archive.add(file, arcname=file.relative_to(artifacts).as_posix())
    rows.append((evidence_archive.name, digest(evidence_archive)))
    (output / "SHA256SUMS").write_text("".join(f"{sha}  {name}\n" for name, sha in sorted(rows)))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--artifacts", type=Path, required=True)
    parser.add_argument("--output", type=Path, required=True)
    parser.add_argument("--source-commit", required=True)
    parser.add_argument("--require-admitted", action="store_true")
    args = parser.parse_args()
    stage(args.artifacts, args.output, args.source_commit, args.require_admitted)


if __name__ == "__main__":
    main()
