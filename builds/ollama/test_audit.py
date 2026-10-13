"""Policy tests for compiled-input audit helpers; fixtures are not release evidence."""
import json
from pathlib import Path
import subprocess
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
SCRIPT = ROOT / "builds/ollama/files/audit-runner.py"
TARGETS = ("linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64")


class AuditPolicy(unittest.TestCase):
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

    def test_inventory_requires_every_target_and_generated_metal(self):
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
            bindir = Path(td) / "bin"; bindir.mkdir()
            fake = bindir / "llvm-readobj"
            fake.write_text("#!/bin/sh\ncase \"$*\" in *darwin*) lib=/usr/lib/libSystem.B.dylib ;; *windows*) lib=KERNEL32.dll ;; *) lib=libc.so.6 ;; esac\nprintf 'NeededLibraries [\\n  %s\\n]\\n' \"$lib\"\n")
            fake.chmod(0o755)
            import os
            env = dict(os.environ, PATH=str(bindir) + os.pathsep + os.environ.get("PATH", ""))
            self.run_audit("inventory", "--out", out, "--report", out / "linked.json", env=env)
            data = json.loads((out / "linked.json").read_text())
            self.assertEqual({x["target"] for x in data["targets"]}, set(TARGETS))
            inputs = json.loads((out / "runner-darwin-arm64.compiler-inputs.json").read_text())
            inputs["files"].pop("ml/backend/ggml/ggml-metal.metal")
            (out / "runner-darwin-arm64.compiler-inputs.json").write_text(json.dumps(inputs))
            self.assertIn("generated Metal", self.run_audit("inventory", "--out", out, "--report", out / "linked.json", env=env, ok=False).stderr)

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


if __name__ == "__main__":
    unittest.main()
