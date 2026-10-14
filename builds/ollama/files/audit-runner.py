#!/usr/bin/env python3
"""Fail-closed compiled-input audit helpers for the official runner recipe."""
import argparse
import csv
import hashlib
import json
from pathlib import Path
import re

TARGETS = ("linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64")
PERMISSIVE = {"MIT", "Apache-2.0", "BSD-2-Clause", "BSD-3-Clause"}
SYSTEM_LIBS = {
    "linux": {"libc.so.6", "libdl.so.2", "libgcc_s.so.1", "libm.so.6", "libpthread.so.0", "libresolv.so.2", "librt.so.1", "libstdc++.so.6"},
    "darwin": {"/usr/lib/libSystem.B.dylib", "/System/Library/Frameworks/Accelerate.framework/Versions/A/Accelerate", "/System/Library/Frameworks/Metal.framework/Versions/A/Metal", "/System/Library/Frameworks/Foundation.framework/Versions/C/Foundation", "/System/Library/Frameworks/CoreFoundation.framework/Versions/A/CoreFoundation", "/System/Library/Frameworks/IOKit.framework/Versions/A/IOKit", "/System/Library/Frameworks/QuartzCore.framework/Versions/A/QuartzCore", "/System/Library/Frameworks/MetalKit.framework/Versions/A/MetalKit", "/System/Library/Frameworks/MetalPerformanceShaders.framework/Versions/A/MetalPerformanceShaders", "/System/Library/Frameworks/CoreVideo.framework/Versions/A/CoreVideo", "/System/Library/Frameworks/CoreGraphics.framework/Versions/A/CoreGraphics", "/System/Library/Frameworks/Security.framework/Versions/A/Security", "/System/Library/Frameworks/AppKit.framework/Versions/C/AppKit", "/System/Library/Frameworks/AudioToolbox.framework/Versions/A/AudioToolbox"},
    "windows": {"advapi32.dll", "bcrypt.dll", "crypt32.dll", "kernel32.dll", "ntdll.dll", "ole32.dll", "oleaut32.dll", "shell32.dll", "user32.dll", "ws2_32.dll", "winmm.dll", "winhttp.dll", "version.dll", "secur32.dll"},
}


def fail(message):
    raise SystemExit("audit-runner: " + message)


def load_json(path):
    try:
        return json.loads(Path(path).read_text())
    except (OSError, json.JSONDecodeError) as exc:
        fail(f"cannot read valid JSON {path}: {exc}")


def go_packages(path):
    text = Path(path).read_text()
    decoder = json.JSONDecoder(); offset = 0; packages = []
    while offset < len(text):
        while offset < len(text) and text[offset].isspace(): offset += 1
        if offset == len(text): break
        try: item, offset = decoder.raw_decode(text, offset)
        except json.JSONDecodeError as exc: fail(f"invalid go list JSON stream {path}: {exc}")
        if isinstance(item, dict) and item.get("ImportPath"): packages.append(item)
    if not packages:
        fail(f"compiled Go package graph is empty: {path}")
    return packages


def ldd_libraries(path, stem):
    # Parse the build-time ldd record (Linux native tool). An unmapped
    # dependency ('=> not found') or an unparseable line fails closed: the
    # record must describe a complete, resolvable runtime closure.
    libs = []
    for raw in Path(path).read_text().splitlines():
        line = raw.strip()
        if not line:
            continue
        mapped = re.fullmatch(r"(\S+) => (\S+) \(0x[0-9a-fA-F]+\)", line)
        direct = re.fullmatch(r"(\S+) \(0x[0-9a-fA-F]+\)", line)
        if mapped:
            if mapped.group(2) == "not found":
                fail(f"{stem}: unmapped shared dependency: {mapped.group(1)}")
            libs.append(mapped.group(1))
        elif direct:
            libs.append(direct.group(1))
        else:
            fail(f"{stem}: malformed ldd linkage record: {line}")
    if not libs:
        fail(f"{stem}: empty Linux linkage record")
    return libs


def otool_libraries(path, stem):
    # Parse the build-time otool -L record (Darwin native tool): a binary
    # header line, then one versioned dylib per line.
    lines = Path(path).read_text().splitlines()
    if not lines or not lines[0].rstrip().endswith(":"):
        fail(f"{stem}: malformed otool linkage record (missing binary header)")
    libs = []
    for raw in lines[1:]:
        line = raw.strip()
        if not line:
            continue
        match = re.fullmatch(r"(\S+) \(compatibility version [^)]+, current version [^)]+\)", line)
        if not match:
            fail(f"{stem}: malformed otool linkage entry: {line}")
        libs.append(match.group(1))
    if not libs:
        fail(f"{stem}: empty Darwin linkage record")
    return libs


def windows_dlls(path, stem):
    # Parse the build-time llvm-objdump DLL-import record (Windows native
    # tool via the pinned BASHY_WINDOWS_OBJDUMP): one '== binary ==' header
    # per shipped executable, then one DLL name per line.
    dlls = []
    seen_header = False
    for raw in Path(path).read_text().splitlines():
        line = raw.strip()
        if not line:
            continue
        if line.startswith("== ") and line.endswith(" =="):
            seen_header = True
            continue
        if not re.fullmatch(r"[A-Za-z0-9_.+-]+\.dll", line, re.IGNORECASE):
            fail(f"{stem}: malformed DLL import record: {line}")
        dlls.append(line)
    if not seen_header or not dlls:
        fail(f"{stem}: empty or headerless Windows DLL record")
    return dlls


def linked_libraries(out, stem, family):
    # Link-aware inventory: each platform's build-time record is parsed by
    # its own format rules and adjudicated against the platform allowlist.
    # No single cross-format reader: llvm-readobj --needed-libs has no
    # NeededLibraries table for Mach-O/PE (CI runs 38448516095 and later).
    if family in ("linux", "darwin"):
        record = out / f"{stem}-linkage.txt"
        if not record.is_file() or not record.stat().st_size:
            fail(f"{stem}: native linkage record missing: {record.name}")
        libs = ldd_libraries(record, stem) if family == "linux" else otool_libraries(record, stem)
        if family == "linux":
            unknown = sorted({lib for lib in libs if lib != "linux-vdso.so.1"
                              and Path(lib).name not in SYSTEM_LIBS["linux"]
                              and not Path(lib).name.startswith("ld-linux")})
        else:
            unknown = sorted(set(libs) - SYSTEM_LIBS["darwin"])
    else:
        record = out / f"{stem}-dlls.txt"
        if not record.is_file() or not record.stat().st_size:
            fail(f"{stem}: native DLL record missing: {record.name}")
        libs = windows_dlls(record, stem)
        allowed = {name.lower() for name in SYSTEM_LIBS["windows"]}
        unknown = sorted({lib for lib in set(libs) if lib.lower() not in allowed})
    if unknown:
        fail(f"{stem}: unreviewed non-platform linked libraries: {', '.join(unknown)}")
    return sorted(set(libs))


def inventory(out, report):
    rows = []
    for target in TARGETS:
        stem = f"runner-{target}"
        inputs = load_json(out / f"{stem}.inputs.json")
        if inputs.get("schema") != "bashy-runner-source-inputs-v1" or not inputs.get("files"):
            fail(f"{stem}: source input inventory missing or has wrong schema")
        if any(not isinstance(path, str) or not isinstance(digest, str) or not re.fullmatch(r"[0-9a-f]{64}", digest) for path, digest in inputs["files"].items()):
            fail(f"{stem}: malformed compiled-source input path or SHA-256")
        native = load_json(out / f"{stem}.compiler-inputs.json")
        if native.get("schema") != "bashy-runner-compiler-inputs-v1" or not native.get("files") or not native.get("invocations"):
            fail(f"{stem}: actual cgo compiler invocations/dependencies are missing")
        if any(not re.fullmatch(r"[0-9a-f]{64}", digest) for digest in native["files"].values()):
            fail(f"{stem}: malformed native compiler-input digest")
        # Build-time Metal generation is an input to the arm64 Darwin artifact.
        if target == "darwin-arm64":
            generated = [p for p in native["files"] if "ggml-metal" in p and
                         (p.endswith(".metal") or p.endswith(".metallib") or p.endswith(".metal.go") or p.endswith(".metal.h"))]
            if not generated:
                fail("darwin-arm64: generated Metal shader input is absent from the compiled-input inventory")
        deps = out / f"{stem}.deps.json"
        packages = go_packages(deps)
        binary = out / stem
        if target == "windows-amd64":
            binary = out / (stem + ".exe")
        if not binary.is_file() or not binary.stat().st_size:
            fail(f"{stem}: native executable missing")
        libs = linked_libraries(out, stem, target.split("-")[0])
        modules = sorted({(p.get("Module") or {}).get("Path", "") + "@" + (p.get("Module") or {}).get("Version", "") for p in packages if (p.get("Module") or {}).get("Path")})
        rows.append({"target": target, "executable": binary.name, "deps_inventory": deps.name, "compiled_modules": modules,
                     "source_inputs": inputs["schema"], "linked_libraries": sorted(set(libs)),
                     "native_compiler_inputs": f"{stem}.compiler-inputs.json",
                     "linkage_policy": "platform system libraries only; no third-party native library linked"})
    Path(report).write_text(json.dumps({"schema": "bashy-runner-linked-libraries-v1", "targets": rows}, indent=2, sort_keys=True) + "\n")


def merge_inputs(out):
    # Require and verify compiler inputs from every native build before
    # auditing (prima.cpp merge-inputs.py pattern): each target's reviewed
    # file manifest must resolve, byte-for-byte, to the capture content
    # blobs downloaded from that target's build leg. A missing or tampered
    # blob fails the audit before any license review reads the manifest.
    for target in TARGETS:
        stem = f"runner-{target}"
        native = load_json(out / f"{stem}.compiler-inputs.json")
        if native.get("schema") != "bashy-runner-compiler-inputs-v1" or not native.get("files"):
            fail(f"{stem}: compiler-input manifest missing or has wrong schema")
        if not native.get("invocations") or not native.get("dependency_files"):
            fail(f"{stem}: compiler invocation evidence is missing")
        store = out / f"{stem}.compiler-inputs" / "content"
        for path, digest in native["files"].items():
            if not re.fullmatch(r"[0-9a-f]{64}", digest):
                fail(f"{stem}: malformed native compiler-input digest: {path}")
            blob = store / digest
            try:
                data = blob.read_bytes()
            except OSError:
                fail(f"{stem}: compiler-input blob missing from downloaded build records: {path}")
            if hashlib.sha256(data).hexdigest() != digest:
                fail(f"{stem}: compiler-input blob hash mismatch: {path}")


def compiler_inputs(capture, source, generated, output):
    capture, source = Path(capture), Path(source).resolve()
    logs = sorted((capture / "invocations").glob("*.json"))
    if not logs: fail("cgo compiler invocation log is missing")
    try:
        invocations = [json.loads(log.read_text()) for log in logs]
    except (json.JSONDecodeError, OSError) as exc:
        fail(f"malformed compiler invocation log: {exc}")
    captured = [row for row in invocations if row.get("captured") is True and row.get("returncode") == 0]
    depfiles = sorted((capture / "depfiles").glob("*.d"))
    if any(row.get("error") for row in invocations): fail("compiler capture failed")
    if not captured: fail("successful compiler input evidence is missing")
    if any(not Path(row["depfile"]).is_file() for row in captured):
        fail("compiler dependency file is missing")
    import importlib.util
    spec = importlib.util.spec_from_file_location("evidence", Path(__file__).with_name("compiler-evidence.py"))
    evidence = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(evidence)
    paths, system_headers, trusted = {}, set(), []
    for row in captured:
        if row.get("error") or not row.get("cwd") or not row.get("inputs"):
            fail("missing compile-time input evidence or failed capture")
        if evidence.dependencies(Path(row["depfile"])) != [item["name"] for item in row["inputs"]]:
            fail("dependency closure differs from compile-time evidence")
        for item in row["inputs"]:
            digest = item["sha256"]
            if not re.fullmatch(r"[0-9a-f]{64}", digest): fail("invalid content digest")
            blob = capture / "content" / digest
            if not blob.is_file() or hashlib.sha256(blob.read_bytes()).hexdigest() != digest:
                fail("missing or corrupt compile-time content")
            try:
                category, rel = evidence.origin(item["path"], row["roots"])
            except ValueError as exc: fail(str(exc))
            if (category, rel) != (item["origin"], item["relative"]): fail("input origin mismatch")
            if Path(row["roots"]["BASHY_COMPILER_SOURCE"]).resolve() != source:
                fail("capture source root mismatch")
            if category == "source":
                if rel in paths and paths[rel] != digest: fail("compiler input changed between invocations")
                paths[rel] = digest
            elif category == "system": system_headers.add(item["path"])
            elif category in ("go-generated", "go-toolchain"): trusted.append(item)
            else: fail("unreviewed external native compiler dependency")
    # Metal source is generated after go generate and may not appear in a C depfile.
    metal_root = Path(generated)
    for p in metal_root.rglob("*"):
        if p.is_file() and (p.name.endswith(".metal") or p.name.endswith(".metallib") or
                            p.name.endswith(".metal.h") or p.name.endswith(".metal.go")):
            paths[p.resolve().relative_to(source).as_posix()] = hashlib.sha256(p.read_bytes()).hexdigest()
    if not paths: fail("no repository-owned C/C++ compiler dependencies were captured")
    files = dict(sorted(paths.items()))
    data = {"schema": "bashy-runner-compiler-inputs-v1", "files": files,
            "invocations": len(invocations), "dependency_files": len(depfiles),
            "system_headers": sorted(system_headers), "trusted_go_inputs": trusted,
            "compiler_invocations": invocations}
    Path(output).write_text(json.dumps(data, indent=2, sort_keys=True) + "\n")


def sys_platform():
    import sys
    return sys.platform


def review_native_licenses(out, review_path, output):
    review = load_json(review_path)
    if review.get("schema") != "bashy-runner-native-license-review-v1":
        fail("native license review has wrong schema")
    reviewed = review.get("inputs", {})
    result = []
    for target in TARGETS:
        name = f"runner-{target}.compiler-inputs.json"
        native = load_json(Path(out) / name)
        for path, digest in native["files"].items():
            row = reviewed.get(path)
            if not row or row.get("sha256") != digest:
                fail(f"unreviewed native compiler input (exact hash required): {target}:{path}")
            license_id = row.get("license")
            if license_id not in PERMISSIVE:
                fail(f"non-permissive or unknown native compiler license for {path}: {license_id}")
            notice = row.get("notice")
            if not isinstance(notice, str) or not notice.strip():
                fail(f"explicit notice missing for reviewed native compiler input: {path}")
            result.append({"target": target, "path": path, "sha256": digest,
                           "license": license_id, "notice": notice})
    Path(output).write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1",
        "policy": "exact SHA-256 match; MIT/BSD/Apache-2.0 only; explicit notice required", "inputs": result},
        indent=2, sort_keys=True) + "\n")


def licenses(report, notices, output):
    notice_files = [p for p in Path(notices).rglob("*") if p.is_file() and p.stat().st_size]
    if not notice_files:
        fail("license notices are empty; refusing to claim license clearance")
    entries = []
    with open(report, newline="") as stream:
        for row in csv.reader(stream):
            if len(row) != 2 or not row[0] or not row[1]:
                fail(f"malformed/unresolved Go license report row: {row!r}")
            name, license_id = row
            ids = {x.strip() for x in re.split(r"[;|]", license_id)}
            if not ids or not ids <= PERMISSIVE:
                fail(f"non-permissive or unresolved compiled Go license for {name}: {license_id}")
            entries.append({"package": name, "license": license_id})
    if not entries:
        fail("compiled Go license report is empty")
    Path(output).write_text(json.dumps({"schema": "bashy-runner-license-review-v1", "policy": "MIT/BSD/Apache-2.0 only", "packages": entries}, indent=2, sort_keys=True) + "\n")


def aggregate_sbom(out):
    path = out / "runner-sbom.cdx.json"
    bom = load_json(path)
    lic = load_json(out / "runner-licenses.json")
    licenses_by_package = {x["package"]: x["license"] for x in lic.get("packages", [])}
    modules = {}
    for target in TARGETS:
        for package in go_packages(out / f"runner-{target}.deps.json"):
            module = package.get("Module") or {}
            module_path, version = module.get("Path"), module.get("Version")
            if not module_path or not version: continue
            key = (module_path, version)
            entry = modules.setdefault(key, set())
            entry.add(target)
    for (module, version), targets in sorted(modules.items()):
        purl = f"pkg:golang/{module}@{version}"
        target_list = ",".join(sorted(targets))
        old = next((x for x in bom.get("components", []) if x.get("purl") == purl), None)
        if old is not None:
            props = old.setdefault("properties", [])
            props[:] = [p for p in props if p.get("name") != "bashy:compiled-targets"]
            props.append({"name": "bashy:compiled-targets", "value": target_list})
            continue
        candidates = [(name, value) for name, value in licenses_by_package.items() if name == module or name.startswith(module + "/")]
        if not candidates:
            fail(f"CycloneDX cannot resolve a reviewed license for compiled module {module}@{version}")
        license_id = sorted(candidates, key=lambda row: len(row[0]), reverse=True)[0][1]
        bom.setdefault("components", []).append({"type": "library", "name": module, "version": version, "purl": purl,
            "licenses": [{"license": {"id": license_id}}], "properties": [{"name": "bashy:compiled-targets", "value": target_list}]})
    linked = load_json(out / "runner-linked-libraries.json")
    for target in linked["targets"]:
        for lib in target["linked_libraries"]:
            key = ("platform:" + lib, target["target"])
            bom.setdefault("components", []).append({"type": "library", "group": "platform-provided", "name": lib,
                "properties": [{"name": "bashy:target", "value": target["target"]}, {"name": "bashy:origin", "value": "operating-system SDK/runtime; not bundled"}]})
    native_review = load_json(out / "runner-native-licenses.json")
    for item in native_review.get("inputs", []):
        bom.setdefault("components", []).append({"type": "file", "name": item["path"],
            "hashes": [{"alg": "SHA-256", "content": item["sha256"]}],
            "licenses": [{"license": {"id": item["license"]}}],
            "properties": [{"name": "bashy:compiled-target", "value": item["target"]},
                           {"name": "bashy:notice", "value": item["notice"]}]})
    bom.setdefault("properties", []).append({"name": "bashy:compiled-targets", "value": ",".join(TARGETS)})
    bom["components"].sort(key=lambda x: (x.get("purl", ""), x.get("name", ""), str(x.get("properties", []))))
    path.write_text(json.dumps(bom, indent=2, sort_keys=True) + "\n")


def no_findings(report, tool, predicate):
    data = load_json(report)
    found = predicate(data)
    if found:
        fail(f"{tool}: unresolved vulnerability findings ({len(found)}); see {report}")


def osv(report):
    def findings(data):
        found = []
        def walk(value):
            if isinstance(value, dict):
                vulns = value.get("vulnerabilities")
                if isinstance(vulns, list):
                    found.extend(vulns)
                for child in value.values(): walk(child)
            elif isinstance(value, list):
                for child in value: walk(child)
        walk(data)
        return found
    no_findings(report, "OSV-Scanner", findings)


def grype(report):
    no_findings(report, "Grype", lambda d: d.get("matches", []))


def final(out, commit):
    required = ["runner-linked-libraries.json", "runner-licenses.json", "runner-native-licenses.json", "runner-sbom.cdx.json", "runner-osv.json", "runner-grype.json"]
    for name in required:
        if not (out / name).is_file() or (out / name).stat().st_size == 0:
            fail(f"required audit evidence missing: {name}")
    linked = load_json(out / "runner-linked-libraries.json")
    if linked.get("schema") != "bashy-runner-linked-libraries-v1" or {x.get("target") for x in linked.get("targets", [])} != set(TARGETS):
        fail("native linked-library inventory does not cover exactly all five targets")
    for target in TARGETS:
        report = out / f"runner-{target}.govulncheck.txt"
        if not report.is_file() or report.stat().st_size == 0:
            fail(f"govulncheck binary reachability report missing for {target}")
    sbom = load_json(out / "runner-sbom.cdx.json")
    if sbom.get("bomFormat") != "CycloneDX" or not sbom.get("components"):
        fail("CycloneDX SBOM is empty or invalid")
    notices = out / "notices"
    notice_files = [p for p in notices.rglob("*") if p.is_file() and p.stat().st_size]
    if not notice_files:
        fail("third-party notices are missing")
    native_review = load_json(out / "runner-native-licenses.json")
    if native_review.get("schema") != "bashy-runner-native-license-review-v1":
        fail("native compiler license review has wrong schema")
    expected_native = {}
    for target in TARGETS:
        native = load_json(out / f"runner-{target}.compiler-inputs.json")
        for path, digest in native.get("files", {}).items():
            expected_native[(target, path, digest)] = None
    for item in native_review.get("inputs", []):
        key = (item.get("target"), item.get("path"), item.get("sha256"))
        if key not in expected_native or expected_native[key] is not None:
            fail(f"native license review has duplicate or extraneous input: {key[0]}:{key[1]}")
        if item.get("license") not in PERMISSIVE or not isinstance(item.get("notice"), str) or not item["notice"].strip():
            fail(f"native license review has unresolved license or notice: {key[1]}")
        expected_native[key] = True
    unresolved = [f"{target}:{path}" for (target, path, _), value in expected_native.items() if value is None]
    if unresolved:
        fail("native license review omitted compiled inputs: " + ", ".join(unresolved[:8]))
    native_notices = "\n\n".join(f"{x['path']} (SHA-256 {x['sha256']}, {x['license']})\n{x['notice']}"
                                    for x in native_review.get("inputs", []))
    (out / "runner-NOTICES.txt").write_text("Notices generated from compiled Go and native inputs.\n\n" +
        "\n".join(p.read_text(errors="replace") for p in sorted(notice_files)) +
        ("\n\nNative compiler-input notices:\n" + native_notices if native_notices else ""))
    vulnerability_record = {"schema": "bashy-runner-vulnerability-review-v1", "upstream_commit": commit,
        "policy": "zero OSV SBOM findings, zero Grype SBOM findings, and zero govulncheck reachable findings in each shipped binary",
        "osv_report": "runner-osv.json", "grype_report": "runner-grype.json",
        "govulncheck_reports": [f"runner-{target}.govulncheck.txt" for target in TARGETS],
        "unresolved_findings": 0}
    (out / "runner-vulnerabilities.json").write_text(json.dumps(vulnerability_record, indent=2, sort_keys=True) + "\n")
    if not commit or not re.fullmatch(r"[0-9a-f]{40}", commit):
        fail("expected official upstream commit was not supplied")
    print(f"compiled-input audit complete for {commit}: five binaries, five Go dependency records, exact native-input license review, native linkage, vulnerabilities, SBOM, and notices")


def main():
    p = argparse.ArgumentParser()
    sub = p.add_subparsers(dest="mode", required=True)
    q = sub.add_parser("inventory"); q.add_argument("--out", type=Path, required=True); q.add_argument("--report", type=Path, required=True)
    q = sub.add_parser("merge-inputs"); q.add_argument("--out", type=Path, required=True)
    q = sub.add_parser("compiler-inputs"); q.add_argument("--capture", required=True); q.add_argument("--source", required=True); q.add_argument("--generated", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("native-licenses"); q.add_argument("--out", required=True); q.add_argument("--review", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("licenses"); q.add_argument("--report", required=True); q.add_argument("--notices", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("osv"); q.add_argument("--report", required=True)
    q = sub.add_parser("grype"); q.add_argument("--report", required=True)
    q = sub.add_parser("aggregate-sbom"); q.add_argument("--out", type=Path, required=True)
    q = sub.add_parser("final"); q.add_argument("--out", type=Path, required=True); q.add_argument("--commit", required=True)
    a = p.parse_args()
    {"inventory": lambda: inventory(a.out, a.report), "merge-inputs": lambda: merge_inputs(a.out),
     "compiler-inputs": lambda: compiler_inputs(a.capture, a.source, a.generated, a.output),
     "native-licenses": lambda: review_native_licenses(a.out, a.review, a.output), "licenses": lambda: licenses(a.report, a.notices, a.output),
     "osv": lambda: osv(a.report), "grype": lambda: grype(a.report), "aggregate-sbom": lambda: aggregate_sbom(a.out), "final": lambda: final(a.out, a.commit)}[a.mode]()


if __name__ == "__main__":
    main()
