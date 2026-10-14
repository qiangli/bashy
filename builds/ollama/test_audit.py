"""Policy tests for compiled-input audit helpers; fixtures are not release evidence."""
import hashlib
import json
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import importlib.util

sys.dont_write_bytecode = True

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "builds/ollama/files/audit-runner.py"
TARGETS = ("linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64")
spec = importlib.util.spec_from_file_location("audit_runner", SCRIPT)
audit_runner = importlib.util.module_from_spec(spec)
spec.loader.exec_module(audit_runner)


class AuditPolicy(unittest.TestCase):
    def test_linkage_parsers_are_format_strict(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            ldd = root / "linux.txt"
            ldd.write_text("linux-vdso.so.1 (0x00007fffc9d5c000)\n"
                           "libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007F2B6C200000)\n"
                           "\t/lib64/ld-linux-x86-64.so.2 (0x00007f2b6c5c0000)\n")
            self.assertEqual(audit_runner.ldd_libraries(ldd, "runner-linux-amd64"),
                             ["linux-vdso.so.1", "libc.so.6", "/lib64/ld-linux-x86-64.so.2"])
            for bad, hint in [("libc.so.6 => not found (0x00007f2b6c200000)", "unmapped"),
                              ("libc.so.6 => /lib/libc.so.6", "malformed"),
                              ("not a dynamic executable", "malformed"),
                              ("", "empty")]:
                ldd.write_text(bad + "\n" if bad else "")
                with self.subTest(report=bad), self.assertRaises(SystemExit):
                    audit_runner.ldd_libraries(ldd, "runner-linux-amd64")
            otool = root / "darwin.txt"
            otool.write_text("out/runner-darwin-arm64:\n"
                             "\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)\n")
            self.assertEqual(audit_runner.otool_libraries(otool, "runner-darwin-arm64"),
                             ["/usr/lib/libSystem.B.dylib"])
            for bad in ["\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)\n",
                        "out/runner-darwin-arm64:\n\t/usr/lib/libSystem.B.dylib\n",
                        "out/runner-darwin-arm64:\n"]:
                otool.write_text(bad)
                with self.subTest(report=bad), self.assertRaises(SystemExit):
                    audit_runner.otool_libraries(otool, "runner-darwin-arm64")
            dlls = root / "windows.txt"
            dlls.write_text("== out/runner-windows-amd64.exe ==\nKERNEL32.dll\nws2_32.dll\n")
            self.assertEqual(audit_runner.windows_dlls(dlls, "runner-windows-amd64"),
                             ["KERNEL32.dll", "ws2_32.dll"])
            for bad in ["KERNEL32.dll\n",
                        "== out/runner-windows-amd64.exe ==\n",
                        "== out/runner-windows-amd64.exe ==\nkernel32\n"]:
                dlls.write_text(bad)
                with self.subTest(report=bad), self.assertRaises(SystemExit):
                    audit_runner.windows_dlls(dlls, "runner-windows-amd64")
    def run_audit(self, *args, ok=True, env=None):
        p = subprocess.run(["python3", str(SCRIPT), *map(str, args)], text=True, capture_output=True, env=env)
        if ok and p.returncode:
            self.fail(p.stdout + p.stderr)
        if not ok and not p.returncode:
            self.fail("unexpected audit success")
        return p

    def test_license_report_fails_closed_and_accepts_permissive_only(self):
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            notices = root / "notices"; notices.mkdir(); (notices / "MIT.txt").write_text("MIT notice")
            report = root / "licenses.csv"; output = root / "licenses.json"
            report.write_text("example.org/dep,M	I\n")
            self.assertIn("unresolved", self.run_audit("licenses", "--report", report, "--notices", notices, "--output", output, ok=False).stderr)
            report.write_text("example.org/dep,MIT\n")
            self.run_audit("licenses", "--report", report, "--notices", notices, "--output", output)
            # MIT-0 (elected for miniaudio's dual Unlicense OR MIT-0,
            # S436/#2028) is permissive; the Unlicense side is not admitted.
            report.write_text("example.org/miniaudio,MIT-0\n")
            self.run_audit("licenses", "--report", report, "--notices", notices, "--output", output)
            report.write_text("example.org/miniaudio,Unlicense\n")
            self.assertIn("non-permissive", self.run_audit("licenses", "--report", report, "--notices", notices, "--output", output, ok=False).stderr)
            report.write_text("example.org/dep,GPL-3.0\n")
            self.assertIn("non-permissive", self.run_audit("licenses", "--report", report, "--notices", notices, "--output", output, ok=False).stderr)

    def test_vulnerability_reports_fail_on_findings_and_empty_is_clear(self):
        with tempfile.TemporaryDirectory() as td:
            report = Path(td) / "scan.json"
            report.write_text(json.dumps({"results": []})); self.run_audit("osv", "--report", report)
            report.write_text(json.dumps({"results": [{"packages": [{"vulnerabilities": [{"id": "OSV-1"}]}]}]}))
            self.assertIn("findings", self.run_audit("osv", "--report", report, ok=False).stderr)
            report.write_text(json.dumps({"matches": []})); self.run_audit("grype", "--report", report)
            report.write_text(json.dumps({"matches": [{"vulnerability": {"id": "CVE-1"}}]}))
            self.assertIn("findings", self.run_audit("grype", "--report", report, ok=False).stderr)

    def test_osv_gate_honours_vuln_ignore_by_exact_id(self):
        # S436/#2028: the one reviewed waiver (GO-2026-5932, unreachable
        # deprecated x/crypto openpgp) excuses exactly that id; every other
        # finding still fails, including near-miss ids.
        with tempfile.TemporaryDirectory() as td:
            report = Path(td) / "scan.json"
            waived = {"results": [{"packages": [{"vulnerabilities": [{"id": "GO-2026-5932"}]}]}]}
            report.write_text(json.dumps(waived)); self.run_audit("osv", "--report", report)
            mixed = {"results": [{"packages": [{"vulnerabilities": [{"id": "GO-2026-5932"}, {"id": "GO-2026-0000"}]}]}]}
            report.write_text(json.dumps(mixed))
            self.assertIn("findings", self.run_audit("osv", "--report", report, ok=False).stderr)
            near_miss = {"results": [{"packages": [{"vulnerabilities": [{"id": "GO-2026-593"}]}]}]}
            report.write_text(json.dumps(near_miss))
            self.assertIn("findings", self.run_audit("osv", "--report", report, ok=False).stderr)
            # An explicit alternate ignore file is honoured the same way, and
            # only for the ids it lists.
            ignore = Path(td) / "ignore"
            ignore.write_text("# comment\n\nGO-2026-0000 waived here\n")
            other = {"results": [{"packages": [{"vulnerabilities": [{"id": "GO-2026-0000"}]}]}]}
            report.write_text(json.dumps(other))
            self.run_audit("osv", "--report", report, "--ignore", ignore)
            self.assertIn("findings", self.run_audit("osv", "--report", report, ok=False).stderr)
            # A waiver file the gate cannot read exactly fails closed.
            ignore.write_text("not-an-id\n")
            self.assertIn("malformed", self.run_audit("osv", "--report", report, "--ignore", ignore, ok=False).stderr)

    def test_govulncheck_gate_fails_only_on_unwaived_reachable(self):
        # S436/#2028 standing verdict rule (same as
        # scripts/govulncheck-release.sh and builds/_shared/kinds/go/audit.yml):
        # REACHABLE (the trace's first frame names a function) and unlisted
        # fails; REACHABLE and waived passes; module-level and package-level
        # findings are reported and pass even when unlisted (GO-2026-5750
        # stays out of vuln-ignore for exactly this reason). Reports the gate
        # cannot read exactly fail closed.
        waived = ["GO-2025-4251", "GO-2025-3824", "GO-2025-3695", "GO-2025-3689",
                  "GO-2025-3559", "GO-2025-3582", "GO-2025-3558", "GO-2025-3557"]
        mod = "github.com/ollama/ollama"
        def trace(level):
            if level == "reachable":
                return [{"module": mod, "version": "v0.21.2",
                         "package": mod + "/llama", "function": "Accept"}]
            if level == "package":
                return [{"module": mod, "version": "v0.21.2",
                         "package": mod + "/llama"}]
            return [{"module": mod, "version": "v0.21.2"}]
        def write_reports(out, by_target):
            for target in TARGETS:
                docs = [{"config": {"scanner_name": "govulncheck", "scan_mode": "binary"}}]
                for vid, level in by_target.get(target, []):
                    docs.append({"finding": {"osv": vid, "trace": trace(level)}})
                (out / f"runner-{target}.govulncheck.json").write_text(
                    "\n".join(json.dumps(d, indent=2) for d in docs) + "\n")
        def summary_of(out):
            return json.loads((out / "runner-govulncheck.json").read_text())
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out"; out.mkdir()
            # Reachable and waived on every target passes, with a JSON summary.
            write_reports(out, {t: [(v, "reachable") for v in waived] for t in TARGETS})
            self.run_audit("govulncheck", "--out", out)
            summary = summary_of(out)
            self.assertEqual(summary["schema"], "bashy-runner-govulncheck-review-v1")
            self.assertEqual(set(summary["targets"]), set(TARGETS))
            self.assertEqual(summary["targets"]["linux-amd64"]["unwaived"], [])
            self.assertEqual(set(summary["targets"]["linux-amd64"]["waived"]), set(waived))
            # A clean scan (no findings at all) also passes.
            write_reports(out, {})
            self.run_audit("govulncheck", "--out", out)
            # One reachable unlisted id on one target fails, naming id and target.
            write_reports(out, {"linux-amd64": [("GO-2026-0000", "reachable")]})
            p = self.run_audit("govulncheck", "--out", out, ok=False)
            self.assertIn("GO-2026-0000", p.stderr)
            self.assertIn("linux-amd64", p.stderr)
            # The JSON summary is still written on failure.
            self.assertEqual(summary_of(out)["targets"]["linux-amd64"]["unwaived"], ["GO-2026-0000"])
            # Near-miss ids are not waived: exact match only.
            write_reports(out, {"linux-amd64": [("GO-2025-425", "reachable")]})
            self.assertIn("findings", self.run_audit("govulncheck", "--out", out, ok=False).stderr)
            # Module-level unlisted is reported and passes (GO-2026-5750).
            write_reports(out, {"darwin-arm64": [("GO-2026-5750", "module")]})
            p = self.run_audit("govulncheck", "--out", out)
            self.assertIn("reported GO-2026-5750 darwin-arm64 module", p.stdout)
            summary = summary_of(out)
            self.assertEqual(summary["targets"]["darwin-arm64"]["reported"],
                             [{"id": "GO-2026-5750", "level": "module"}])
            self.assertEqual(summary["targets"]["darwin-arm64"]["unwaived"], [])
            # Package-level unlisted is reported and passes.
            write_reports(out, {"linux-amd64": [("GO-2026-0000", "package")]})
            p = self.run_audit("govulncheck", "--out", out)
            self.assertIn("reported GO-2026-0000 linux-amd64 package", p.stdout)
            self.assertEqual(summary_of(out)["targets"]["linux-amd64"]["reported"],
                             [{"id": "GO-2026-0000", "level": "package"}])
            # Advisory metadata alone neither excuses nor accuses.
            with tempfile.TemporaryDirectory() as td2:
                out2 = Path(td2) / "out"; out2.mkdir()
                for target in TARGETS:
                    (out2 / f"runner-{target}.govulncheck.json").write_text(
                        json.dumps({"osv": {"id": "GO-2026-0000"}}) + "\n")
                self.run_audit("govulncheck", "--out", out2)
            # A waiver file the gate cannot read exactly fails closed.
            ignore = Path(td) / "ignore"
            ignore.write_text("not-an-id\n")
            write_reports(out, {})
            self.assertIn("malformed", self.run_audit(
                "govulncheck", "--out", out, "--ignore", ignore, ok=False).stderr)
            # An explicit alternate ignore file is honoured the same way.
            ignore.write_text("# comment\n\nGO-2026-0000 waived here\n")
            write_reports(out, {"linux-amd64": [("GO-2026-0000", "reachable")]})
            self.run_audit("govulncheck", "--out", out, "--ignore", ignore)
            # A --targets subset replays a partial local scan (e.g. the two
            # -12 binaries); unknown targets fail closed.
            write_reports(out, {"darwin-arm64": [("GO-2026-5750", "module")]})
            p = self.run_audit("govulncheck", "--out", out, "--targets", "darwin-arm64")
            self.assertIn("reported GO-2026-5750 darwin-arm64 module", p.stdout)
            self.assertEqual(set(summary_of(out)["targets"]), {"darwin-arm64"})
            self.assertIn("subset", self.run_audit(
                "govulncheck", "--out", out, "--targets", "plan9-amd64", ok=False).stderr)

    def test_govulncheck_gate_fails_closed_on_unreadable_reports(self):
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out"; out.mkdir()
            self.assertIn("cannot read",
                          self.run_audit("govulncheck", "--out", out, ok=False).stderr)
            for target in TARGETS:
                (out / f"runner-{target}.govulncheck.json").write_text(
                    "" if target == "linux-amd64" else "{}\n")
            p = self.run_audit("govulncheck", "--out", out, ok=False)
            self.assertIn("empty", p.stderr)
            (out / "runner-linux-amd64.govulncheck.json").write_text("{truncated\n")
            self.assertIn("invalid", self.run_audit("govulncheck", "--out", out, ok=False).stderr)
            (out / "runner-linux-amd64.govulncheck.json").write_text('[{"finding": 1}]\n')
            self.assertIn("malformed", self.run_audit("govulncheck", "--out", out, ok=False).stderr)
            (out / "runner-linux-amd64.govulncheck.json").write_text(
                json.dumps({"finding": {"trace": []}}) + "\n")
            self.assertIn("malformed", self.run_audit("govulncheck", "--out", out, ok=False).stderr)

    def test_inventory_requires_every_target_and_generated_metal(self):
        # Link-aware inventory: each target's build-time linkage record is
        # reviewed in its native format. No llvm-readobj on PATH is needed.
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out"; out.mkdir()
            for target in TARGETS:
                stem = "runner-" + target
                files = {"cmd/runner/main.go": "a" * 64}
                native_files = {"vendor/native/runner.c": "c" * 64}
                if target == "darwin-arm64": native_files["ml/backend/ggml/ggml-metal.metal"] = "b" * 64
                (out / (stem + ".inputs.json")).write_text(json.dumps({"schema": "bashy-runner-source-inputs-v1", "files": files}))
                (out / (stem + ".compiler-inputs.json")).write_text(json.dumps({"schema": "bashy-runner-compiler-inputs-v1", "files": native_files, "invocations": 1}))
                (out / (stem + ".deps.json")).write_text(json.dumps({"ImportPath": "example.org/dep", "Module": {"Path": "example.org/dep", "Version": "v1.0.0"}}) + "\n")
                (out / (stem + (".exe" if target == "windows-amd64" else ""))).write_bytes(b"fixture")
                family = target.split("-")[0]
                if family == "linux":
                    (out / (stem + "-linkage.txt")).write_text(
                        "linux-vdso.so.1 (0x00007fffc9d5c000)\n"
                        "libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f2b6c200000)\n"
                        "/lib64/ld-linux-x86-64.so.2 (0x00007f2b6c5c0000)\n")
                elif family == "darwin":
                    (out / (stem + "-linkage.txt")).write_text(
                        f"out/{stem}:\n"
                        "\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1351.0.0)\n")
                else:
                    (out / (stem + "-dlls.txt")).write_text(f"== out/{stem}.exe ==\nKERNEL32.dll\nws2_32.dll\n")
            self.run_audit("inventory", "--out", out, "--report", out / "linked.json")
            data = json.loads((out / "linked.json").read_text())
            self.assertEqual({x["target"] for x in data["targets"]}, set(TARGETS))
            by_target = {x["target"]: x for x in data["targets"]}
            self.assertIn("libc.so.6", by_target["linux-amd64"]["linked_libraries"])
            self.assertIn("/usr/lib/libSystem.B.dylib", by_target["darwin-arm64"]["linked_libraries"])
            self.assertIn("KERNEL32.dll", by_target["windows-amd64"]["linked_libraries"])
            (out / "runner-linux-amd64-linkage.txt").write_text(
                "libevil.so.1 => /opt/evil/libevil.so.1 (0x00007f2b6c200000)\n")
            self.assertIn("unreviewed", self.run_audit("inventory", "--out", out, "--report", out / "linked.json", ok=False).stderr)
            (out / "runner-linux-amd64-linkage.txt").write_text(
                "libc.so.6 => /lib/x86_64-linux-gnu/libc.so.6 (0x00007f2b6c200000)\n")
            (out / "runner-windows-amd64-dlls.txt").unlink()
            self.assertIn("DLL record missing", self.run_audit("inventory", "--out", out, "--report", out / "linked.json", ok=False).stderr)
            (out / "runner-windows-amd64-dlls.txt").write_text("== out/runner-windows-amd64.exe ==\nKERNEL32.dll\n")
            inputs = json.loads((out / "runner-darwin-arm64.compiler-inputs.json").read_text())
            inputs["files"].pop("ml/backend/ggml/ggml-metal.metal")
            (out / "runner-darwin-arm64.compiler-inputs.json").write_text(json.dumps(inputs))
            self.assertIn("generated Metal", self.run_audit("inventory", "--out", out, "--report", out / "linked.json", ok=False).stderr)

    def test_merge_inputs_verifies_downloaded_blobs(self):
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out"; out.mkdir()
            for target in TARGETS:
                stem = "runner-" + target
                body = f"input for {target}".encode()
                digest = hashlib.sha256(body).hexdigest()
                (out / (stem + ".compiler-inputs.json")).write_text(json.dumps(
                    {"schema": "bashy-runner-compiler-inputs-v1", "files": {"vendor/native/a.c": digest},
                     "invocations": 1, "dependency_files": 1}))
                store = out / (stem + ".compiler-inputs") / "content"; store.mkdir(parents=True)
                (store / digest).write_bytes(body)
            self.run_audit("merge-inputs", "--out", out)
            victim = "runner-linux-amd64"
            blob = next((out / (victim + ".compiler-inputs") / "content").glob("*"))
            blob.write_text("tampered")
            self.assertIn("hash mismatch", self.run_audit("merge-inputs", "--out", out, ok=False).stderr)
            blob.unlink()
            self.assertIn("blob missing", self.run_audit("merge-inputs", "--out", out, ok=False).stderr)

    def test_compiler_inputs_stores_generated_metal_blob_for_merge(self):
        # Generated Metal shaders never appear in a C depfile, so
        # compiler_inputs must persist their bytes into the
        # content-addressed store; otherwise merge_inputs fails the audit
        # with a missing blob for every manifest entry.
        with tempfile.TemporaryDirectory() as td:
            root = Path(td)
            source = root / "source"; source.mkdir()
            (source / "a.c").write_bytes(b"int x;")
            metal_rel = "ml/backend/ggml/ggml/src/ggml-metal/ggml-metal-embed.metal"
            metal = source / metal_rel
            metal.parent.mkdir(parents=True)
            metal.write_bytes(b"metal shader bytes")
            capture = root / "capture"
            (capture / "invocations").mkdir(parents=True)
            (capture / "depfiles").mkdir(parents=True)
            (capture / "content").mkdir(parents=True)
            digest_a = hashlib.sha256(b"int x;").hexdigest()
            (capture / "content" / digest_a).write_bytes(b"int x;")
            src_abs = str((source / "a.c").resolve())
            dep = capture / "depfiles" / "a.d"
            dep.write_text(f"obj.o: {src_abs}\n")
            row = {"captured": True, "returncode": 0, "cwd": str(source),
                   "depfile": str(dep),
                   "roots": {"BASHY_COMPILER_SOURCE": str(source.resolve())},
                   "inputs": [{"name": src_abs, "path": src_abs, "origin": "source",
                               "relative": "a.c", "sha256": digest_a}]}
            (capture / "invocations" / "a.json").write_text(json.dumps(row))
            manifest = root / "out.json"
            audit_runner.compiler_inputs(str(capture), str(source), str(source), str(manifest))
            data = json.loads(manifest.read_text())
            metal_digest = hashlib.sha256(b"metal shader bytes").hexdigest()
            self.assertEqual(data["files"][metal_rel], metal_digest)
            blob = capture / "content" / metal_digest
            self.assertTrue(blob.is_file())
            self.assertEqual(blob.read_bytes(), b"metal shader bytes")
            out = root / "mout"; out.mkdir()
            for target in TARGETS:
                stem = "runner-" + target
                (out / (stem + ".compiler-inputs.json")).write_text(json.dumps(data))
                store = out / (stem + ".compiler-inputs") / "content"
                store.mkdir(parents=True)
                for name in (capture / "content").iterdir():
                    (store / name.name).write_bytes(name.read_bytes())
            self.run_audit("merge-inputs", "--out", out)
            ((out / "runner-linux-amd64.compiler-inputs" / "content") / metal_digest).unlink()
            self.assertIn("blob missing", self.run_audit("merge-inputs", "--out", out, ok=False).stderr)

    def test_native_license_review_requires_exact_hash_permissive_id_and_notice(self):
        with tempfile.TemporaryDirectory() as td:
            out = Path(td)
            for target in TARGETS:
                name = "runner-" + target + ".compiler-inputs.json"
                path = "vendor/native/a.metal" if target == "darwin-arm64" else "vendor/native/a.c"
                files = {path: "a" * 64}
                if target == "darwin-arm64": files["vendor/native/a.c"] = "a" * 64
                (out / name).write_text(json.dumps({"schema": "bashy-runner-compiler-inputs-v1", "files": files}))
            review = out / "review.json"
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {}}))
            args = ("native-licenses", "--out", out, "--review", review, "--output", out / "licenses.json")
            self.assertIn("exact hash", self.run_audit(*args, ok=False).stderr)
            path = "vendor/native/a.c"
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {path: {"sha256": "a" * 64, "license": "GPL-3.0", "notice": "GPL notice"}}}))
            self.assertIn("non-permissive", self.run_audit(*args, ok=False).stderr)
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {path: {"sha256": "b" * 64, "license": "MIT", "notice": "MIT notice"}}}))
            self.assertIn("exact hash", self.run_audit(*args, ok=False).stderr)
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {path: {"sha256": "a" * 64, "license": "MIT"}, "vendor/native/a.metal": {"sha256": "a" * 64, "license": "MIT", "notice": "MIT notice"}}}))
            self.assertIn("explicit notice", self.run_audit(*args, ok=False).stderr)
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {path: {"sha256": "a" * 64, "license": "MIT", "notice": "MIT notice"}, "vendor/native/a.metal": {"sha256": "a" * 64, "license": "MIT", "notice": "MIT notice"}}}))
            self.run_audit(*args)
            # MIT-0 (elected for miniaudio, S436/#2028) passes the same gate.
            review.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": {path: {"sha256": "a" * 64, "license": "MIT-0", "notice": "MIT-0 elected notice"}, "vendor/native/a.metal": {"sha256": "a" * 64, "license": "MIT", "notice": "MIT notice"}}}))
            self.run_audit(*args)
            generated_review = {path: {"sha256": "a" * 64, "license": "MIT", "notice": "MIT notice"}}
            generated_review_path = out / "generated-review.json"
            generated_review_path.write_text(json.dumps({"schema": "bashy-runner-native-license-review-v1", "inputs": generated_review}))
            self.assertIn("darwin-arm64:vendor/native/a.metal",
                          self.run_audit("native-licenses", "--out", out, "--review",
                                         generated_review_path,
                                         "--output", out / "licenses.json", ok=False).stderr)

    def test_cyclonedx_aggregates_all_target_module_graphs_and_native_links(self):
        with tempfile.TemporaryDirectory() as td:
            out = Path(td)
            (out / "runner-sbom.cdx.json").write_text(json.dumps({"bomFormat": "CycloneDX", "components": []}))
            (out / "runner-licenses.json").write_text(json.dumps({"packages": [{"package": "example.org/dep/pkg", "license": "MIT"}]}))
            (out / "runner-native-licenses.json").write_text(json.dumps({"inputs": []}))
            rows = []
            for target in TARGETS:
                (out / f"runner-{target}.deps.json").write_text(json.dumps({"ImportPath": "example.org/dep/pkg", "Module": {"Path": "example.org/dep", "Version": "v1.2.3"}}) + "\n")
                rows.append({"target": target, "linked_libraries": ["libc.so.6"]})
            (out / "runner-linked-libraries.json").write_text(json.dumps({"targets": rows}))
            self.run_audit("aggregate-sbom", "--out", out)
            bom = json.loads((out / "runner-sbom.cdx.json").read_text())
            component = next(c for c in bom["components"] if c.get("purl") == "pkg:golang/example.org/dep@v1.2.3")
            self.assertEqual(set(component["properties"][0]["value"].split(",")), set(TARGETS))
            self.assertEqual(sum(1 for c in bom["components"] if c["name"] == "libc.so.6"), len(TARGETS))


    def test_inventory_accepts_ci38537053528_platform_libraries(self):
        # CI run 38537053528 linked Apple system libraries and the UCRT API
        # sets that the old allowlist did not know; all are platform
        # libraries, so the inventory must accept them by exact name.
        self.assertIn("/usr/lib/libc++.1.dylib", audit_runner.SYSTEM_LIBS["darwin"])
        self.assertIn("/usr/lib/libresolv.9.dylib", audit_runner.SYSTEM_LIBS["darwin"])
        self.assertIn("/usr/lib/libobjc.A.dylib", audit_runner.SYSTEM_LIBS["darwin"])
        expected_crt = {
            "api-ms-win-crt-convert-l1-1-0.dll", "api-ms-win-crt-environment-l1-1-0.dll",
            "api-ms-win-crt-filesystem-l1-1-0.dll", "api-ms-win-crt-heap-l1-1-0.dll",
            "api-ms-win-crt-locale-l1-1-0.dll", "api-ms-win-crt-math-l1-1-0.dll",
            "api-ms-win-crt-multibyte-l1-1-0.dll", "api-ms-win-crt-private-l1-1-0.dll",
            "api-ms-win-crt-runtime-l1-1-0.dll", "api-ms-win-crt-stdio-l1-1-0.dll",
            "api-ms-win-crt-string-l1-1-0.dll", "api-ms-win-crt-time-l1-1-0.dll",
            "api-ms-win-crt-utility-l1-1-0.dll",
        }
        self.assertTrue(expected_crt <= audit_runner.SYSTEM_LIBS["windows"],
                        audit_runner.SYSTEM_LIBS["windows"] - expected_crt)
        with tempfile.TemporaryDirectory() as td:
            out = Path(td) / "out"; out.mkdir()
            for target in TARGETS:
                stem = "runner-" + target
                files = {"cmd/runner/main.go": "a" * 64}
                native_files = {"vendor/native/runner.c": "c" * 64}
                if target == "darwin-arm64":
                    native_files["ml/backend/ggml/ggml-metal.metal"] = "b" * 64
                (out / (stem + ".inputs.json")).write_text(json.dumps(
                    {"schema": "bashy-runner-source-inputs-v1", "files": files}))
                (out / (stem + ".compiler-inputs.json")).write_text(json.dumps(
                    {"schema": "bashy-runner-compiler-inputs-v1",
                     "files": native_files, "invocations": 1}))
                (out / (stem + ".deps.json")).write_text(
                    json.dumps({"ImportPath": "example.org/dep",
                                "Module": {"Path": "example.org/dep", "Version": "v1.0.0"}}) + "\n")
                (out / (stem + (".exe" if target == "windows-amd64" else ""))).write_bytes(b"fixture")
                family = target.split("-")[0]
                if family == "linux":
                    (out / (stem + "-linkage.txt")).write_text(
                        "libc.so.6 => /lib/libc.so.6 (0x00007f0000000000)\n")
                elif target == "darwin-arm64":
                    (out / (stem + "-linkage.txt")).write_text(
                        f"out/{stem}:\n"
                        "\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n"
                        "\t/usr/lib/libc++.1.dylib (compatibility version 1.0.0, current version 1.0.0)\n"
                        "\t/usr/lib/libresolv.9.dylib (compatibility version 1.0.0, current version 1.0.0)\n"
                        "\t/usr/lib/libobjc.A.dylib (compatibility version 1.0.0, current version 1.0.0)\n")
                elif target == "darwin-amd64":
                    (out / (stem + "-linkage.txt")).write_text(
                        f"out/{stem}:\n"
                        "\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\n"
                        "\t/usr/lib/libc++.1.dylib (compatibility version 1.0.0, current version 1.0.0)\n"
                        "\t/usr/lib/libresolv.9.dylib (compatibility version 1.0.0, current version 1.0.0)\n")
                else:
                    dlls = "\n".join(["== out/runner-windows-amd64.exe ==", "KERNEL32.dll",
                                      *sorted(expected_crt)]) + "\n"
                    (out / (stem + "-dlls.txt")).write_text(dlls)
            self.run_audit("inventory", "--out", out, "--report", out / "linked.json")
            # No wildcard: a UCRT API set outside the exact 13 is still rejected.
            record = out / "runner-windows-amd64-dlls.txt"
            record.write_text(record.read_text() + "api-ms-win-crt-conio-l1-1-0.dll\n")
            self.assertIn("unreviewed", self.run_audit(
                "inventory", "--out", out, "--report", out / "linked.json", ok=False).stderr)

    def test_windows_dll_record_rejects_export_table_self_reference(self):
        # The runner's own cgo //export table line ('a.out.exe') is not a
        # DLL import: the strict parser must reject it, so only the
        # case-sensitive build-runner.sh grep keeps it out of the record.
        with tempfile.TemporaryDirectory() as td:
            record = Path(td) / "dlls.txt"
            record.write_text("== out/runner-windows-amd64.exe ==\nKERNEL32.dll\na.out.exe\n")
            with self.assertRaises(SystemExit):
                audit_runner.windows_dlls(record, "runner-windows-amd64")
        recipe = (ROOT / "builds/ollama/files/build-runner.sh").read_text()
        self.assertIn("grep 'DLL Name:'", recipe)
        self.assertNotIn("grep -i 'DLL Name:'", recipe)


if __name__ == "__main__":
    unittest.main()
