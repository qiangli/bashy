#!/usr/bin/env python3
"""Fail-closed compiled-input audit helpers for the official runner recipe."""
import argparse
import csv
import hashlib
import json
from pathlib import Path
import re
import subprocess

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
        proc = subprocess.run(["llvm-readobj", "--needed-libs", str(binary)], text=True, capture_output=True)
        if proc.returncode:
            fail(f"cannot read native linked-library table for {stem}: {proc.stderr.strip()}")
        # llvm-readobj emits one `  <library>` line under NeededLibraries.
        libs = []
        for line in proc.stdout.splitlines():
            value = line.strip()
            if value and value not in {"NeededLibraries [", "]"} and not value.endswith(":"):
                libs.append(value)
        if "NeededLibraries [" not in proc.stdout:
            fail(f"{stem}: linked-library table is absent (cannot claim native inventory)")
        family = target.split("-")[0]
        allowed = {name.lower() for name in SYSTEM_LIBS[family]} if family == "windows" else SYSTEM_LIBS[family]
        unknown = sorted(name for name in set(libs) if (name.lower() if family == "windows" else name) not in allowed)
        if unknown:
            fail(f"{stem}: unreviewed non-platform linked libraries: {', '.join(unknown)}")
        modules = sorted({(p.get("Module") or {}).get("Path", "") + "@" + (p.get("Module") or {}).get("Version", "") for p in packages if (p.get("Module") or {}).get("Path")})
        rows.append({"target": target, "executable": binary.name, "deps_inventory": deps.name, "compiled_modules": modules,
                     "source_inputs": inputs["schema"], "linked_libraries": sorted(set(libs)),
                     "native_compiler_inputs": f"{stem}.compiler-inputs.json",
                     "linkage_policy": "platform system libraries only; no third-party native library linked"})
    Path(report).write_text(json.dumps({"schema": "bashy-runner-linked-libraries-v1", "targets": rows}, indent=2, sort_keys=True) + "\n")


def compiler_inputs(capture, source, generated, output):
    capture, source = Path(capture), Path(source).resolve()
    log = capture / "invocations.jsonl"
    if not log.is_file(): fail("cgo compiler invocation log is missing")
    try:
        invocations = [json.loads(line) for line in log.read_text().splitlines() if line]
    except (json.JSONDecodeError, OSError) as exc:
        fail(f"malformed compiler invocation log: {exc}")
    captured = [row for row in invocations if row.get("captured") is True]
    depfiles = sorted((capture / "depfiles").glob("*.d"))
    if len(captured) != len(depfiles):
        fail(f"captured cgo compile/dependency-file count differs: {len(captured)} vs {len(depfiles)}")
    if not invocations or not depfiles: fail("cgo compiler invocation or dependency files are missing")
    paths, system_headers = set(), set()
    if sys_platform() == "darwin":
        system_roots = ("/usr/include/", "/System/Library/Frameworks/",
                        "/Library/Developer/CommandLineTools/SDKs/",
                        "/Library/Developer/CommandLineTools/usr/include/",
                        "/Library/Developer/CommandLineTools/usr/lib/clang/",
                        "/Applications/Xcode.app/Contents/Developer/Platforms/",
                        "/Applications/Xcode.app/Contents/Developer/Toolchains/")
    elif sys_platform() == "linux":
        system_roots = ("/usr/include/", "/usr/lib/gcc/", "/usr/lib/clang/", "/usr/local/lib/clang/")
    else:
        system_roots = ()
    for dep in depfiles:
        try:
            raw = dep.read_text(errors="strict").replace("\\\r\n", " ").replace("\\\n", " ")
        except (OSError, UnicodeError) as exc:
            fail(f"cannot read compiler dependency file {dep.name}: {exc}")
        match = re.search(r"(?<!\\):\s", raw)
        if not match: fail(f"malformed compiler dependency file: {dep.name}")
        body = raw[match.end():]
        names, token, escaped = [], [], False
        for char in body:
            if escaped:
                token.append(char); escaped = False
            elif char == "\\": escaped = True
            elif char.isspace():
                if token: names.append("".join(token)); token = []
            else: token.append(char)
        if escaped: token.append("\\")
        if token: names.append("".join(token))
        for name in names:
            p = Path(name)
            if not p.is_absolute(): p = source / p
            resolved = p.resolve()
            if not resolved.is_file(): fail(f"compiler dependency is missing: {name}")
            try:
                rel = resolved.relative_to(source).as_posix()
                paths.add(rel)
            except ValueError:
                external = str(resolved)
                if external.startswith(system_roots):
                    system_headers.add(external)
                else:
                    fail(f"unreviewed external native compiler dependency: {external}")
    # Metal source is generated after go generate and may not appear in a C depfile.
    metal_root = Path(generated)
    for p in metal_root.rglob("*"):
        if p.is_file() and (p.name.endswith(".metal") or p.name.endswith(".metallib") or
                            p.name.endswith(".metal.h") or p.name.endswith(".metal.go")):
            paths.add(p.resolve().relative_to(source).as_posix())
    if not paths: fail("no repository-owned C/C++ compiler dependencies were captured")
    files = {p: hashlib.sha256((source / p).read_bytes()).hexdigest() for p in sorted(paths)}
    data = {"schema": "bashy-runner-compiler-inputs-v1", "files": files,
            "invocations": len(invocations), "dependency_files": len(depfiles),
            "system_headers": sorted(system_headers)}
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
    q = sub.add_parser("compiler-inputs"); q.add_argument("--capture", required=True); q.add_argument("--source", required=True); q.add_argument("--generated", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("native-licenses"); q.add_argument("--out", required=True); q.add_argument("--review", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("licenses"); q.add_argument("--report", required=True); q.add_argument("--notices", required=True); q.add_argument("--output", required=True)
    q = sub.add_parser("osv"); q.add_argument("--report", required=True)
    q = sub.add_parser("grype"); q.add_argument("--report", required=True)
    q = sub.add_parser("aggregate-sbom"); q.add_argument("--out", type=Path, required=True)
    q = sub.add_parser("final"); q.add_argument("--out", type=Path, required=True); q.add_argument("--commit", required=True)
    a = p.parse_args()
    {"inventory": lambda: inventory(a.out, a.report), "compiler-inputs": lambda: compiler_inputs(a.capture, a.source, a.generated, a.output),
     "native-licenses": lambda: review_native_licenses(a.out, a.review, a.output), "licenses": lambda: licenses(a.report, a.notices, a.output),
     "osv": lambda: osv(a.report), "grype": lambda: grype(a.report), "aggregate-sbom": lambda: aggregate_sbom(a.out), "final": lambda: final(a.out, a.commit)}[a.mode]()


if __name__ == "__main__":
    main()
