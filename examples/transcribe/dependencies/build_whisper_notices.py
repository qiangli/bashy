#!/usr/bin/env python3
"""Build an offline, source-pinned notice candidate for a Whisper binary.

This records evidence and copies upstream notices. It is not a legal approval.
"""

import argparse
import gzip
import hashlib
import io
import json
import tarfile
from pathlib import Path, PureWindowsPath


ROOT = Path(__file__).resolve().parent / "notices"
PINS = ROOT / "source-pins.json"
PROJECT_ARCHIVES = {"ggml-base.a", "ggml-cpu.a", "ggml.a", "libcommon.a", "libwhisper.a"}
RUNTIME_ARCHIVES = {"libc++.a", "libunwind.a", "libmingw32.a", "libmingwex.a", "libmsvcrt.a"}
STARTUP_OBJECTS = {"crt2.o", "crtbegin.o", "crtend.o", "cli.cpp.obj"}
LINUX_PROJECT_ARCHIVES = {"libggml-base.a", "libggml-cpu.a", "libggml.a", "libcommon.a", "libwhisper.a"}
LINUX_RUNTIME_ARCHIVES = {"libstdc++.a", "libgcc.a", "libgcc_eh.a"}


def checked(path, expected_sha, expected_size=None):
    data = Path(path).read_bytes()
    if hashlib.sha256(data).hexdigest() != expected_sha or (expected_size is not None and len(data) != expected_size):
        raise ValueError(f"size or SHA-256 mismatch: {path}")
    return data


def digest(data):
    return hashlib.sha256(data).hexdigest()


def load_json(path):
    return json.loads(Path(path).read_text(encoding="utf-8"))


def require(condition, message):
    if not condition:
        raise ValueError(message)


def archive_notice(tar, row):
    """Read the exact notice bytes from the already SHA-verified source archive."""
    member = tar.extractfile(row["archive_member"])
    require(member is not None, "notice source member missing")
    data = member.read()
    if "start_marker" not in row:
        return data
    start = row["start_marker"].encode("utf-8")
    end = row["end_marker"].encode("utf-8")
    require(data.count(start) == 1, "notice start marker missing or ambiguous")
    begin = data.index(start)
    finish = data.find(end, begin)
    require(finish >= 0 and end.startswith(b"\n"), "notice end marker missing or invalid")
    return data[begin:finish + 1]


def generate(args, pins_path=PINS):
    pins = load_json(pins_path)
    source_dir = Path(pins_path).parent / "sources"
    archive = checked(args.source_archive, pins["whisper"]["archive_sha256"], pins["whisper"]["archive_bytes"])
    with tarfile.open(fileobj=io.BytesIO(archive), mode="r:gz") as tar:
        archive_notices = {name: archive_notice(tar, row)
                           for name, row in pins["notices"].items() if row.get("archive_member")}
    notices = {name: checked(source_dir / name, row["sha256"])
               for name, row in pins["notices"].items()}
    for name, data in archive_notices.items():
        require(data == notices[name], f"{name} differs from pinned archive")

    report = load_json(args.build_report)
    target = report["platform"]
    os_name, arch = target.split("/")
    require(os_name in {"windows", "linux", "darwin"} and arch in {"amd64", "arm64"}, "unsupported target")
    require(report["whisper_version"] == pins["whisper"]["version"], "Whisper version mismatch")
    require(report["source_archive_sha256"] == pins["whisper"]["archive_sha256"] and
            report["source_archive_bytes"] == pins["whisper"]["archive_bytes"], "source archive report mismatch")
    require(report["product_admitted"] is False and report["status"] == "built_and_cached", "build status mismatch")
    binary = checked(args.binary, report["executable_sha256"], report["executable_bytes"])
    require(Path(args.binary).name == ("whisper-cli.exe" if os_name == "windows" else "whisper-cli"), "binary name mismatch")

    evidence = {"build_report_sha256": digest(Path(args.build_report).read_bytes())}
    gaps = ["Map linked Whisper and ggml objects to exact source files and check file-level license notices before admission."]
    selected = [name for name, row in pins["notices"].items()
                if row["targets"] == "all" or (os_name == "windows" and row["targets"] == "windows")]
    if os_name == "windows":
        require(all((args.toolchain_report, args.inventory, args.link_map)), "Windows toolchain, inventory and map required")
        tc = load_json(args.toolchain_report)
        asset = pins["windows_toolchain"]["assets"][arch]
        require(tc["platform"] == target and tc["version"] == pins["windows_toolchain"]["tag"] and
                tc["source_url"] == asset["url"] and tc["archive_sha256"] == asset["archive_sha256"] and
                tc["archive_bytes"] == asset["archive_bytes"], "toolchain pin mismatch")
        root = PureWindowsPath(tc["toolchain_root"])
        require(PureWindowsPath(report["compiler_path"]) == root / "bin" / "clang.exe" and
                PureWindowsPath(report["cxx_compiler_path"]) == root / "bin" / "clang++.exe", "compiler path mismatch")
        inventory = load_json(args.inventory)
        require(inventory == report["link_map_inventory"], "inventory differs from build report")
        map_bytes = Path(args.link_map).read_bytes()
        if Path(args.link_map).suffix == ".gz":
            evidence["compressed_map_sha256"] = digest(map_bytes)
            map_bytes = gzip.decompress(map_bytes)
        require(len(map_bytes) == inventory["map_bytes"] and digest(map_bytes) == inventory["map_sha256"], "link map mismatch")
        require(inventory["binary_sha256"] == digest(binary) and inventory["binary_name"] == Path(args.binary).name,
                "inventory binary mismatch")
        require(inventory["archive_members"] and set(inventory["unclassified_archives"]) ==
                PROJECT_ARCHIVES - {"libwhisper.a"}, "unexpected unclassified archives")
        runtime = set(RUNTIME_ARCHIVES)
        runtime.add("libclang_rt.builtins-" + ("x86_64" if arch == "amd64" else "aarch64") + ".a")
        archives = {row["archive"] for row in inventory["archive_members"]}
        require(archives == PROJECT_ARCHIVES | runtime, "unexpected or missing linked archive")
        unmatched = {Path(row.replace("\\", "/")).name for row in inventory["map_objects_without_archive_match"]}
        require(unmatched == STARTUP_OBJECTS, "unclassified startup object")
        evidence.update({"toolchain_report_sha256": digest(Path(args.toolchain_report).read_bytes()),
                         "inventory_sha256": digest(Path(args.inventory).read_bytes()),
                         "map_sha256": digest(map_bytes), "toolchain_asset": asset,
                         "linked_archive_members": len(inventory["archive_members"]),
                         "linked_archives": sorted(archives), "startup_objects": sorted(unmatched)})
        gaps.extend(["MinGW runtime notice itself flags Cephes math source licensing as uncertain; verify linked member source lineage.",
                     "Verify source-to-prebuilt llvm-mingw archive provenance and all linked MinGW member licenses before product admission."])
    elif os_name == "linux":
        require(args.inventory and args.link_map, "Linux inventory and map required")
        inventory = load_json(args.inventory)
        require(inventory == report.get("link_map_inventory"), "inventory differs from build report")
        require(inventory.get("schema") == "s503-linux-link-map-v1", "Linux inventory schema mismatch")
        map_bytes = Path(args.link_map).read_bytes()
        require(0 < len(map_bytes) <= 32 << 20 and len(map_bytes) == inventory.get("map_bytes")
                and digest(map_bytes) == inventory.get("map_sha256"), "link map mismatch")
        require(inventory.get("binary_sha256") == digest(binary) and inventory.get("binary_name") == Path(args.binary).name,
                "inventory binary mismatch")
        members = inventory.get("archive_members", [])
        runtime = inventory.get("runtime_archive_members", [])
        require(members and runtime and all(row in members for row in runtime), "Linux runtime inventory incomplete")
        archives = {Path(row["archive"]).name for row in members}
        require(LINUX_PROJECT_ARCHIVES <= archives and archives <= LINUX_PROJECT_ARCHIVES | LINUX_RUNTIME_ARCHIVES,
                "unexpected or missing linked Linux archive")
        require({Path(row["archive"]).name for row in runtime} == archives & LINUX_RUNTIME_ARCHIVES,
                "Linux runtime archive classification mismatch")
        evidence.update({"inventory_sha256": digest(Path(args.inventory).read_bytes()),
                         "map_sha256": digest(map_bytes), "linked_archive_members": len(members),
                         "linked_archives": sorted(archives), "runtime_archive_members": len(runtime)})
        gaps.append("GCC runtime exception unresolved: inspect exact GPLv3 GCC Runtime Library Exception applicability and source notices for retained static members, or rebuild with host-provided dynamic libstdc++ and verify imports.")
    else:
        gaps.append("Verify macOS linked system framework and runtime inventory and applicable host license notices before product admission.")

    if args.review:
        review = load_json(args.review)
        match = next((row for row in review["targets"] if row["target"] == target), None)
        require(match is not None and match["binary_sha256"] == digest(binary) and match["binary_bytes"] == len(binary)
                and match["product_admitted"] is False, "retained review does not match binary")
        for name, path in (("build-report.json", args.build_report), ("toolchain-report.json", args.toolchain_report),
                           ("whisper-link-inventory.json", args.inventory)):
            require(path and digest(Path(path).read_bytes()) == match["raw_files_sha256"][name], f"review {name} mismatch")
        require(evidence.get("map_sha256") == match["map_sha256"], "review map mismatch")
        if Path(args.link_map).suffix == ".gz":
            require(evidence["compressed_map_sha256"] == match["compressed_map_sha256"], "review compressed map mismatch")
        evidence.update({"review_sha256": digest(Path(args.review).read_bytes()), "run_url": review["run_url"]})

    output = Path(args.output_dir)
    require(not output.exists() and not output.is_symlink(), "output already exists")
    output.mkdir(parents=True)
    license_dir = output / "LICENSES"
    license_dir.mkdir()
    sections = ["Whisper executable third-party notices\n",
                "Candidate notice bundle; product admission and license review remain open.\n"]
    notice_records = []
    for name in selected:
        row = pins["notices"][name]
        data = notices[name]
        (license_dir / name).write_bytes(data)
        sections.append(f"\n===== {name} =====\nSource: {row['url']}\nSHA-256: {row['sha256']}\n\n")
        sections.append(data.decode("utf-8"))
        if not sections[-1].endswith("\n"):
            sections.append("\n")
        notice_records.append({"file": f"LICENSES/{name}", "source_url": row["url"], "sha256": row["sha256"]})
    sections.append("\nOpen review gates:\n" + "".join(f"- {gap}\n" for gap in gaps))
    bundle = "".join(sections).encode("utf-8")
    (output / "THIRD_PARTY_NOTICES.txt").write_bytes(bundle)
    manifest = {"schema": "s503-whisper-notice-bundle-v1", "status": "candidate_product_not_admitted",
                "target": target, "binary_sha256": digest(binary), "binary_bytes": len(binary),
                "source_archive_url": pins["whisper"]["archive_url"],
                "source_archive_sha256": pins["whisper"]["archive_sha256"],
                "source_pins_sha256": digest(Path(pins_path).read_bytes()), "evidence": evidence,
                "notices": notice_records, "bundle_sha256": digest(bundle), "open_gates": gaps}
    (output / "NOTICE_MANIFEST.json").write_text(json.dumps(manifest, indent=2, sort_keys=True) + "\n", encoding="utf-8")
    return manifest


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    for key in ("build-report", "binary", "source-archive", "output-dir"):
        parser.add_argument("--" + key, required=True, type=Path)
    for key in ("toolchain-report", "inventory", "link-map", "review"):
        parser.add_argument("--" + key, type=Path)
    args = parser.parse_args()
    print(json.dumps(generate(args), indent=2, sort_keys=True))


if __name__ == "__main__":
    main()
