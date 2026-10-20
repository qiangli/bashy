#!/usr/bin/env python3
"""Bounded offline recipe gate. Fake compiler output is never native evidence."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import sys
import tempfile
import unittest

ROOT = Path(__file__).resolve().parents[2]
RECIPE = ROOT / "builds/ollama"
PIN = "590109c8352e8d5a6206e8909b518a54a2b0a7b8"
TARGETS = ["linux-amd64", "linux-arm64", "darwin-amd64", "darwin-arm64", "windows-amd64"]
HEADERS = ["cmd/runner/main.go", "llama/llama.cpp/vendor/nlohmann/json.hpp",
           "llama/llama.cpp/vendor/nlohmann/json_fwd.hpp",
           "llama/llama.cpp/vendor/miniaudio/miniaudio.h",
           "llama/llama.cpp/vendor/stb/stb_image.h",
           "ml/backend/ggml/ggml/src/ggml-metal/metal.go", "llama/llama.cpp/native.c",
           "llama/llama.cpp/native.cpp"]


def run(*args, ok=True, **kwargs):
    p = subprocess.run(args, text=True, capture_output=True, **kwargs)
    if ok and p.returncode:
        raise AssertionError(p.stdout + p.stderr)
    if not ok and not p.returncode:
        raise AssertionError("unexpected success: " + str(args))
    return p


class Recipe(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.fork = self.root / "fork"
        run("bash", str(ROOT / "builds/render.sh"), "ollama", str(self.fork))
        self.helpers = self.fork / "bashy-build"

    def fixture(self):
        src = self.root / "src"
        src.mkdir()
        for name in HEADERS:
            path = src / name
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text("source fixture\n")
        # Minimal module graph for the GO_SECURITY_OVERRIDES gate: the
        # fixture pins the nine x/* modules below the override, so
        # build-runner.sh must bump each to its reviewed version (the
        # derived MVS closure of the x/image fix) before compiling.
        (src / "go.mod").write_text("module example.org/runner\n\ngo 1.24.1\n\nrequire golang.org/x/crypto v0.43.0\nrequire golang.org/x/image v0.22.0\nrequire golang.org/x/mod v0.30.0\nrequire golang.org/x/net v0.46.0\nrequire golang.org/x/sync v0.17.0\nrequire golang.org/x/sys v0.37.0\nrequire golang.org/x/term v0.36.0\nrequire golang.org/x/text v0.30.0\nrequire golang.org/x/tools v0.38.0\nrequire github.com/buger/jsonparser v1.1.1\n")
        (src / "go.sum").write_text("")
        run("git", "init", "-q", str(src))
        run("git", "-C", str(src), "add", ".")
        run("git", "-C", str(src), "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
            "-c", "commit.gpgsign=false", "commit", "-qm", "fixture")
        commit = run("git", "-C", str(src), "rev-parse", "HEAD").stdout.strip()
        run("git", "-C", str(src), "tag", "v0.21.2")
        pin = self.helpers / "PIN"
        pin.write_text(pin.read_text().replace(PIN, commit))
        return src

    def test_render_contract(self):
        run("diff", "-r", str(self.fork), str(RECIPE / "fork"))
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        self.assertEqual(wf.count("repository: ollama/ollama"), 2)
        self.assertIn("shell: bash", wf)
        self.assertIn("needs: [pin, build, audit]", wf)
        self.assertIn('--certificate-identity "$EXPECTED_IDENTITY"', wf)
        self.assertIn('--certificate-oidc-issuer "$OIDC_ISSUER"', wf)
        self.assertIn("subject-checksums: dist/SHA256SUMS", wf)
        audit = wf.split("audit + SBOM", 1)[1].split("  release:", 1)[0]
        for required in ("govulncheck", "osv-scanner", "grype", "CycloneDX", "runner-linked-libraries.json", "runner-licenses.json", "merge-inputs"):
            self.assertIn(required, audit)
        # Grype v0.93.0 cannot migrate today's DB; the pinned install stays
        # at v0.120.1 (Apache-2.0, already license-checked). The pin is the
        # positional TAG: install.sh -v requests cosign signature verification
        # (cosign is not on the runners; the flag fails the install without
        # it), so the step must not pass -v. Tarball SHA-256 verification is
        # unconditional in install.sh.
        self.assertIn("GRYPE_VERSION: v0.120.1", audit)
        self.assertIn("anchore/grype/v0.120.1/install.sh", audit)
        self.assertIn('-b "$RUNNER_TEMP/bin" "$GRYPE_VERSION"', audit)
        self.assertNotIn('-v "$GRYPE_VERSION"', audit)
        self.assertNotIn("v0.93.0", audit)
        self.assertNotIn("llvm-readobj", audit)
        self.assertTrue((self.helpers / "windows-defender-egress.ps1").is_file())
        self.assertFalse((self.helpers / "windows-net-guard.c").exists())
        self.assertIn("needs: [pin, build]", audit)
        self.assertNotIn("unresolved runner security admission", audit)
        self.assertTrue((self.helpers / "audit-runner.py").is_file())
        self.assertIn("runner-NOTICES.txt", (self.helpers / "audit-runner.py").read_text())
        for target in TARGETS:
            self.assertIn("- asset: runner-" + target, wf)
        pin = (self.helpers / "PIN").read_text()
        self.assertIn("UPSTREAM_COMMIT=" + PIN, pin)
        self.assertIn("UPSTREAM_SLUG=ollama/ollama", pin)
        run("bash", str(self.helpers / "no-egress-test.sh"), ok=False)
        # An ordinary host and a self-hosted Actions runner must both stop before
        # fetching a model, probing tools or starting the supplied executable.
        for hosted in ("", "self-hosted"):
            env = dict(os.environ, GITHUB_ACTIONS="true", RUNNER_ENVIRONMENT=hosted)
            refusal = run("bash", str(self.helpers / "no-egress-test.sh"),
                          "nonexistent/runner-linux-amd64", env=env, ok=False)
            self.assertIn("requires a GitHub-hosted runner", refusal.stderr)
        self.assertIn("if: always()", wf)
        self.assertIn("out/*-native.json", wf)
        self.assertIn("out/*-egress.txt", wf)
        self.assertIn("runner-windows-amd64-native.json", pin)

    def test_cuda_target_carries_backend_artifacts_in_manifest(self):
        # S451/#2487: a CUDA lane is a SEPARATE Linux amd64 target. A Go-only
        # runner is not CUDA; the manifest must enumerate every backend shared
        # library with its digest and a verified source/build identity, and
        # each library must be its own signed release asset (never a symlink).
        pin = (self.helpers / "PIN").read_text()
        asset = "runner-linux-amd64-cuda"
        self.assertIn(asset + "=ubuntu-24.04", pin)
        for suffix in ("", ".gz", ".json", ".inputs.json", ".deps.json", "-libggml-cuda.so"):
            self.assertIn(" " + asset + suffix + " ", pin)
        # Existing CPU/Metal targets stay untouched.
        for target in TARGETS:
            self.assertIn("runner-" + target + "=", pin)
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn(asset, build)
        self.assertIn("nvcc", build)
        self.assertIn("backend-artifacts", build)
        package = (self.helpers / "package-runner.py").read_text()
        for key in ("backend_artifacts", "backend_identity", "cuda_toolkit", "source_commit"):
            self.assertIn(key, package)
        self.assertNotIn("ln -s", build)
        self.assertIn("source-guard.sh", pin)

    def test_cuda_manifest_digests_and_audit_hold(self):
        d = self.root / "cuda"
        d.mkdir()
        (d / "runner").write_bytes(b"exe")
        (d / "lib.so").write_bytes(b"backend")
        ident = d / "id.json"
        base = {"source_commit": PIN, "cuda_toolkit": "Build cuda_12.x", "nvcc_sha256": "a" * 64, "toolkit_sha256": "b" * 64,
                "preset": "CUDA 12", "artifacts": ["lib.so"]}
        ident.write_text(json.dumps(base))
        pkg = str(self.helpers / "package-runner.py")
        args = [pkg, "package", str(d / "runner"), str(d / "r.gz"), str(d / "runner-linux-amd64-cuda.json"),
                "linux/amd64", PIN, "1.27.2"]
        run("python3", *args, str(ident))
        m = json.loads((d / "runner-linux-amd64-cuda.json").read_text())
        self.assertEqual(m["backend_artifacts"][0]["sha256"], hashlib.sha256(b"backend").hexdigest())
        self.assertTrue(m["release_admission"].startswith("PENDING_CUDA"))
        ident.write_text(json.dumps(dict(base, source_commit="0" * 40)))
        run("python3", *args, str(ident), ok=False)
        ident.write_text(json.dumps(base))
        (d / "lib.so").unlink()
        (d / "lib.so").symlink_to(d / "runner")
        run("python3", *args, str(ident), ok=False)
        hold = run("python3", str(self.helpers / "audit-runner.py"), "final", "--out", str(d), "--commit", PIN, ok=False)
        self.assertIn("CUDA backend not admitted", hold.stdout + hold.stderr)

    def test_cuda_toolkit_pinned_build_only_and_runtime_host_provided(self):
        # S451.4a operator decision 2026-10-20: the CUDA toolkit is a pinned,
        # sha256-verified BUILD-ONLY SDK recorded in the manifest/SBOM; driver,
        # libcuda, cudart and cuBLAS are host-provided dynamically and never
        # bundled, statically linked or released. Missing runtime fails closed.
        env = (Path(__file__).resolve().parent / "tool.env").read_text()
        for key in ("CUDA_TOOLKIT_VERSION=", "CUDA_TOOLKIT_SHA256="):
            self.assertIn(key, env)
        install = self.helpers / "install-cuda-toolkit.sh"
        self.assertTrue(install.exists(), "pinned toolkit installer missing")
        text = install.read_text()
        self.assertIn("sha256sum", text)
        self.assertIn("CUDA_TOOLKIT_SHA256", text)
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("install-cuda-toolkit.sh", build)
        self.assertIn("CUDA_TOOLKIT_SHA256", build)
        self.assertIn("CUDA_USE_STATIC", build)  # static cudart/cublas link refused
        package = (self.helpers / "package-runner.py").read_text()
        self.assertIn("toolkit_sha256", package)
        self.assertIn("build-only", package)
        self.assertIn("host-provided", package)
        for banned in ("libcudart", "libcublas", "libcuda.so"):
            self.assertIn(banned, package)  # refused as backend artifacts

    def test_scanner_findings_defer_to_audit_gate(self):
        # Run 38544523506: osv-scanner exits 1 on ANY finding (including the
        # reviewed waiver GO-2026-5932), which killed the step under set -e
        # before the audit-runner.py osv gate (which applies vuln-ignore)
        # ran. The scanner exit code must never decide: capture it, accept 0
        # (clean) and 1 (findings written), fail anything else with a clear
        # message, then always run the gate. Grype runs WITHOUT --fail-on
        # (--fail-on exits 2 when matches exceed the threshold, seen in the
        # run-132 replay: scan exit 2 with 32 matches), so it exits 0 with
        # findings written and the audit-runner.py grype gate decides; any
        # nonzero grype exit is a real scanner error.
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        osv = wf.split("OSV scan of the five-target compiled-input SBOM", 1)[1].split("- name:", 1)[0]
        self.assertIn("osv-scanner scan source", osv)
        self.assertIn("|| rc=$?", osv)
        self.assertIn("$rc -ne 0", osv)
        self.assertIn("$rc -ne 1", osv)
        self.assertIn("failed with exit", osv)
        self.assertIn("audit-runner.py osv", osv)
        self.assertLess(osv.index("osv-scanner scan source"), osv.index("audit-runner.py osv"))
        self.assertLess(osv.index("|| rc=$?"), osv.index("audit-runner.py osv"))
        grype = wf.split("Grype complete CycloneDX SBOM", 1)[1].split("- name:", 1)[0]
        invocation = [line for line in grype.splitlines() if "bin/grype" in line]
        self.assertEqual(len(invocation), 1)
        self.assertNotIn("--fail-on", invocation[0])
        self.assertIn("|| rc=$?", grype)
        self.assertIn("$rc -ne 0", grype)
        self.assertNotIn("$rc -ne 1", grype)
        self.assertIn("failed with exit", grype)
        self.assertIn("audit-runner.py grype", grype)
        self.assertLess(grype.index("sbom:out/runner-sbom.cdx.json"), grype.index("audit-runner.py grype"))
        self.assertLess(grype.index("|| rc=$?"), grype.index("audit-runner.py grype"))
        # S436/#2028: govulncheck -mode=binary exits 3 on the waived
        # Ollama-module advisories, which killed the step under set -e before
        # any gate ran. The scanner exit code must never decide: scan with
        # -json per target, accept 0 (clean) and 3 (findings written), fail
        # anything else with a clear message, then always run the
        # audit-runner.py govulncheck gate (which applies vuln-ignore).
        gov = wf.split("govulncheck binary reachability", 1)[1].split("- name:", 1)[0]
        self.assertIn("govulncheck -mode=binary -json", gov)
        self.assertIn("runner-$target.govulncheck.json", gov)
        self.assertIn("|| rc=$?", gov)
        self.assertIn("$rc -ne 0", gov)
        self.assertIn("$rc -ne 3", gov)
        self.assertIn("failed with exit", gov)
        self.assertIn("audit-runner.py govulncheck", gov)
        self.assertLess(gov.index("govulncheck -mode=binary -json"), gov.index("audit-runner.py govulncheck"))
        self.assertLess(gov.index("|| rc=$?"), gov.index("audit-runner.py govulncheck"))

    def test_licenses_step_fits_pinned_go_licenses(self):
        # go-licenses v1.6.0 save creates its directory (a pre-created one
        # fatals with 'already exists'), and report --template takes a
        # template FILE whose rows use the tool's fields (.LicenseName;
        # there is no .License). CI run 38537053528 never reached this step.
        # S436/#2028: the report covers every shipped target — one
        # check/report per GOOS/GOARCH with the rows unioned — so
        # Windows-only compiled modules (mousetrap) reach aggregate-sbom.
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        step = wf.split("compiled-input permissive license review", 1)[1].split("- name:", 1)[0]
        self.assertNotIn("report --template '{{", step)
        self.assertIn('go-licenses report --template "$tmpl"', step)
        self.assertIn(".LicenseName", step)
        for target in TARGETS:
            goos, goarch = target.split("-")
            self.assertIn(f"GOOS=$goos GOARCH=$goarch go-licenses check", step)
            self.assertIn(f"GOOS=$goos GOARCH=$goarch go-licenses report", step)
        self.assertIn("sort -u -o ../out/runner-licenses.csv", step)
        self.assertIn('go-licenses save --save_path="../out/notices-$target"', step)
        self.assertIn("mousetrap", step)
        # Portable first-wins notices merge: bare cp -n exits 1 on macOS
        # when the destination exists (GNU exits 0), which kills the step
        # under set -euo pipefail (replayed on macos-host).
        self.assertNotIn('cp -n "../out/notices-', step)
        self.assertIn("if [[ ! -e $dest ]]", step)

    def test_security_override_applies_in_build_and_audit(self):
        # KIND=custom, so the recipe mirrors the go-kind GO_SECURITY_OVERRIDES
        # mechanism rather than including it: the declaration, the gated
        # helper, and its application after the pin gates in BOTH jobs.
        pin = (self.helpers / "PIN").read_text()
        # The declaration is the derived MVS closure of the x/image fix
        # (nine x/* modules at exactly the selected versions) plus the
        # jsonparser GO-2026-4514 fix (a leaf module: no further MVS moves).
        self.assertIn("golang.org/x/image@v0.47.0", pin)
        for spec in ("golang.org/x/crypto@v0.57.0", "golang.org/x/mod@v0.41.0",
                     "golang.org/x/net@v0.59.0", "golang.org/x/sync@v0.24.0",
                     "golang.org/x/sys@v0.49.0", "golang.org/x/term@v0.46.0",
                     "golang.org/x/text@v0.43.0", "golang.org/x/tools@v0.51.0",
                     "github.com/buger/jsonparser@v1.1.2"):
            self.assertIn(spec, pin)
        self.assertTrue((self.helpers / "go-security-overrides.sh").is_file())
        run("bash", "-n", str(self.helpers / "go-security-overrides.sh"))
        helper = (self.helpers / "go-security-overrides.sh").read_text()
        self.assertIn("go get", helper)
        self.assertIn("go.mod changed outside the require lines", helper)
        build = (RECIPE / "files/build-runner.sh").read_text()
        self.assertLess(build.index("source-guard.sh"), build.index("go-security-overrides.sh"))
        self.assertLess(build.index("go-security-overrides.sh"), build.index("go list -mod=readonly"))
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        audit = wf.split("audit + SBOM", 1)[1].split("  release:", 1)[0]
        self.assertIn("GO_SECURITY_OVERRIDES (reviewed module bumps", audit)
        self.assertLess(audit.index("GO_SECURITY_OVERRIDES (reviewed module bumps"),
                        audit.index("compiled-input permissive license review"))
        # Nine reviewed waivers (S436/#2028): GO-2026-5932, the unreachable
        # deprecated x/crypto openpgp finding with no fixed version, plus the
        # eight Ollama-module advisories with no patched version anywhere
        # (operator-accepted, Sprint 436). The OSV and govulncheck gates
        # honour them by exact id; nothing else is waived.
        self.assertTrue((RECIPE / "files/vuln-ignore").is_file())
        waivers = (RECIPE / "files/vuln-ignore").read_text()
        self.assertIn("GO-2026-5932", waivers)
        for vid in ("GO-2025-4251", "GO-2025-3824", "GO-2025-3695", "GO-2025-3689",
                    "GO-2025-3559", "GO-2025-3582", "GO-2025-3558", "GO-2025-3557"):
            self.assertIn(vid, waivers)
        self.assertEqual(len(re.findall(r"GO-[0-9]+-[0-9]+", waivers)), 9)
        self.assertIn("vuln-ignore", wf)
        gate = (RECIPE / "files/audit-runner.py").read_text()
        self.assertIn("vuln-ignore", gate)
        self.assertIn("vuln_ignored_ids", gate)

    def test_native_license_review_pins_every_compiled_input(self):
        # The committed review must admit exactly the permissive set
        # (including elected MIT-0 for miniaudio) with an exact SHA-256 and
        # an explicit notice per compiled native input.
        import importlib.util
        spec = importlib.util.spec_from_file_location("audit_runner", RECIPE / "files/audit-runner.py")
        audit_runner = importlib.util.module_from_spec(spec)
        prev_flag = sys.dont_write_bytecode
        sys.dont_write_bytecode = True
        try:
            spec.loader.exec_module(audit_runner)
        finally:
            sys.dont_write_bytecode = prev_flag
        review = json.loads((RECIPE / "files/license-review.json").read_text())
        self.assertEqual(review["schema"], "bashy-runner-native-license-review-v1")
        self.assertIn("MIT-0", audit_runner.PERMISSIVE)
        inputs = review["inputs"]
        self.assertGreaterEqual(len(inputs), 300)
        mini = inputs["llama/llama.cpp/vendor/miniaudio/miniaudio.h"]
        self.assertEqual(mini["license"], "MIT-0")
        self.assertIn("MIT-0", mini["notice"])
        self.assertEqual(mini["sha256"], "05b75871465913f1e3f5b05f0dcca1d487f3099e653477e071acc40317e68f38")
        for path, row in inputs.items():
            with self.subTest(path=path):
                self.assertIn(row["license"], audit_runner.PERMISSIVE)
                self.assertRegex(row["sha256"], r"^[0-9a-f]{64}$")
                self.assertTrue(row["notice"].strip())

    def test_no_egress_gate_compiles_nothing(self):
        # The Windows leg enforces via the built-in Defender Firewall: no
        # compiled helper, no WFP link libraries anywhere in the leg.
        script = (RECIPE / "files/no-egress-test.sh").read_text()
        windows = script[script.index("MINGW*|MSYS*)"):]
        for token in ['"$cc" -std=c11', "-lfwpuclnt", "windows-net-guard.c", "net-guard.exe"]:
            self.assertNotIn(token, windows)
        for line in windows.splitlines():
            stripped = line.lstrip()
            self.assertFalse(stripped.startswith("gcc "), line)
        self.assertIn("powershell.exe", windows)
        self.assertIn("windows-defender-egress.ps1", windows)
        # Every evidence marker the shell asserts must be emitted by the
        # Defender script, or the gate can never pass.
        gate = (RECIPE.parent / "_shared/windows-defender-egress.ps1").read_text()
        for marker in ("Defender control blocked", "pfirewall.log", "protocol gate PROTOCOL_PASS",
                       "no non-loopback connections observed",
                       "Defender block rules removed; firewall logging restored"):
            self.assertIn(marker, gate)

    def test_darwin_profile_uses_proven_loopback_form(self):
        # Prima-proven Seatbelt form (tag bashy-bashy-prima-0.1.2-13, run
        # 38486571631, green on the hosted macos-15 runners): (remote ip
        # "localhost:PORT"). The ip4 variant failed CI runs 38448516095 and
        # later. Reproduced on macos-host (macOS 26.3): the ip form
        # compiles, allows the configured loopback port, and denies
        # wrong-port and external endpoints (Seatbelt rejects numeric hosts:
        # "host must be * or localhost in network address").
        script = (RECIPE / "files/no-egress-test.sh").read_text()
        darwin = script[script.index("Darwin)"):script.index("MINGW*|MSYS*)")]
        self.assertIn('(allow network-outbound (remote ip "localhost:$port"))', darwin)
        self.assertNotIn("remote ip4", darwin)
        self.assertNotIn("remote ip6", darwin)

    def test_audit_reviews_build_time_records_without_tool_subprocess(self):
        script = (RECIPE / "files/audit-runner.py").read_text()
        self.assertNotIn("import subprocess", script)
        self.assertNotIn('"llvm-readobj"', script)

    def test_source_guards(self):
        src = self.fixture()
        guard = self.helpers / "source-guard.sh"
        run("bash", str(guard), str(src))
        (src / HEADERS[1]).unlink()
        self.assertIn("not pristine", run("bash", str(guard), str(src), ok=False).stderr)
        run("git", "-C", str(src), "checkout", "--", HEADERS[1])
        (src / HEADERS[0]).write_text("patched entrypoint")
        run("bash", str(guard), str(src), ok=False)
        run("git", "-C", str(src), "checkout", "--", HEADERS[0])
        pin = self.helpers / "PIN"
        original_pin = pin.read_text()
        pin.write_text(original_pin.replace("UPSTREAM_COMMIT=", "UPSTREAM_COMMIT=bad"))
        run("bash", str(guard), str(src), ok=False)
        pin.write_text(original_pin)
        zip_tree = self.root / "module-zip"
        shutil.copytree(src, zip_tree, ignore=shutil.ignore_patterns(".git"))
        run("bash", str(guard), str(zip_tree), ok=False)
        run("git", "-C", str(src), "tag", "-d", "v0.21.2")
        run("bash", str(guard), str(src), ok=False)
        run("git", "-C", str(src), "tag", "v0.21.2")
        # A clean Git tree that lacks nested headers must also fail, even if
        # a future maintainer accidentally pins that incomplete source tree.
        run("git", "-C", str(src), "rm", HEADERS[1])
        run("git", "-C", str(src), "-c", "user.name=Test", "-c", "user.email=test@example.invalid",
            "-c", "commit.gpgsign=false", "commit", "-qm", "incomplete source")
        self.assertIn("!= pinned", run("bash", str(guard), str(src), ok=False).stderr)
        old = run("git", "-C", str(src), "rev-parse", "HEAD^").stdout.strip()
        new = run("git", "-C", str(src), "rev-parse", "HEAD").stdout.strip()
        pin.write_text(original_pin.replace(old, new))
        run("git", "-C", str(src), "tag", "-f", "v0.21.2")
        self.assertIn("missing full Git source input", run("bash", str(guard), str(src), ok=False).stderr)

    def test_native_recipe_and_packaging_with_fake_compiler(self):
        src = self.fixture()
        mock = self.root / "bin"
        mock.mkdir()
        go = mock / "go"
        go.write_text('''#!/usr/bin/env python3
import os, pathlib, shlex, subprocess, sys
args = sys.argv[1:]
assert os.environ['GOWORK'] == 'off'
assert os.environ['CGO_ENABLED'] == '1'
assert os.environ['GOTOOLCHAIN'] == 'local'
if args[0] == 'env':
 print({'GOHOSTOS':os.environ['TEST_HOST'].split('/')[0], 'GOHOSTARCH':os.environ['TEST_HOST'].split('/')[1], 'GOVERSION':'go1.27.2', 'GOROOT':str(pathlib.Path.cwd() / 'goroot')}[args[1]])
elif args[0] == 'build':
 assert args[1:3] == ['-mod=readonly','-trimpath'] and args[-1] == './cmd/runner'
 subprocess.run([*shlex.split(os.environ['CC']), '-c', str(pathlib.Path.cwd() / 'llama/llama.cpp/native.c')], check=True)
 subprocess.run([*shlex.split(os.environ['CXX']), '-c', str(pathlib.Path.cwd() / 'llama/llama.cpp/native.cpp')], check=True)
 pathlib.Path(args[args.index('-o')+1]).write_bytes(b'FAKE compiler bytes: never release')
elif args[0] == 'generate':
 assert os.environ['TEST_HOST'] == 'darwin/arm64'
 assert args[1:] == ['./ml/backend/ggml/ggml/src/ggml-metal']
elif args[0] == 'list':
 print('{}')
elif args[0] == 'mod':
 assert args[1:3] == ['edit', '-json']
 import re, json
 text = pathlib.Path('go.mod').read_text()
 mod = re.search(r'^module\s+(\S+)', text, re.M).group(1)
 mgo = re.search(r'^go\s+(\S+)', text, re.M)
 reqs = [{'Path': m.group(1), 'Version': m.group(2), 'Indirect': False} for m in re.finditer(r'^\s*require\s+(\S+)\s+(\S+)', text, re.M)]
 print(json.dumps({'Module': {'Path': mod}, 'Go': mgo.group(1) if mgo else '', 'Toolchain': '', 'Require': reqs, 'Exclude': [], 'Replace': [], 'Retract': []}))
elif args[0] == 'get':
 import re
 text = pathlib.Path('go.mod').read_text()
 for spec in args[1:]:
  module, version = spec.split('@')
  assert version.startswith('v')
  updated, count = re.subn(r'(?m)^(\s*require\s+' + re.escape(module) + r'\s+)\S+', r'\g<1>' + version, text)
  assert count == 1, spec
  text = updated
 pathlib.Path('go.mod').write_text(text)
else: raise RuntimeError(args)
''')
        go.chmod(0o755)
        import subprocess
        fake_cc = mock / "fake-cc"
        fake_cc.write_text('''#!/usr/bin/env python3
import os, pathlib, sys
args=sys.argv[1:]
if args == ['--version']: print('fake compiler fixture'); sys.exit(0)
if args == ['-dumpmachine']: print('x86_64-w64-windows-gnu' if os.environ.get('GOOS') == 'windows' else 'fixture-target'); sys.exit(0)
if args == ['-dumpfullversion']: print('14.2'); sys.exit(0)
if args[0] == '-dM': print('#define __clang__ 1\\n#define __MINGW64__ 1\\n#define __x86_64__ 1\\n#define __SIZEOF_POINTER__ 8\\n#define __cplusplus 201703L'); sys.exit(0)
assert '-MMD' in args and '-MF' in args
dep=pathlib.Path(args[args.index('-MF')+1])
source=next(x for x in args if x.endswith(('.c', '.cpp')))
dep.write_text('object: ' + source + ' ' + str(pathlib.Path.cwd() / 'llama/llama.cpp/vendor/nlohmann/json.hpp') + '\\n')
''')
        fake_cc.chmod(0o755)
        for name in ("gcc", "g++", "clang", "clang++"):
            (mock / name).symlink_to(fake_cc)
        cygpath = mock / "cygpath"
        cygpath.write_text("#!/usr/bin/env python3\nimport sys\nprint(sys.argv[-1])\n")
        cygpath.chmod(0o755)
        # Native linkage tools are toolchain like the compilers above: the
        # test fakes their output, CI runs the real ldd/otool/llvm-objdump
        # on each native runner.
        ldd = mock / "ldd"
        ldd.write_text('#!/bin/sh\nprintf "linux-vdso.so.1 (0x00007fff00000000)\\nlibc.so.6 => /lib/libc.so.6 (0x00007f0000000000)\\n/lib64/ld-linux-x86-64.so.2 (0x00007f0000000000)\\n"\n')
        ldd.chmod(0o755)
        otool = mock / "otool"
        otool.write_text('#!/bin/sh\necho "$2:"\nprintf "\\t/usr/lib/libSystem.B.dylib (compatibility version 1.0.0, current version 1.0.0)\\n"\n')
        otool.chmod(0o755)
        objdump = mock / "llvm-objdump"
        objdump.write_text('#!/bin/sh\nprintf "DLL Name: KERNEL32.dll\\nDLL Name: WS2_32.dll\\n"\n')
        objdump.chmod(0o755)
        env = dict(os.environ, PATH=str(mock) + os.pathsep + os.environ["PATH"],
                   BASHY_WINDOWS_CC=str(mock / 'gcc'), BASHY_WINDOWS_CXX=str(mock / 'g++'),
                   BASHY_WINDOWS_OBJDUMP=str(mock / 'llvm-objdump'))
        for target in TARGETS:
            with self.subTest(target=target):
                asset = "runner-" + target
                env["TEST_HOST"] = target.replace("-", "/")
                out = self.root / target
                # CI gives every matrix leg a fresh checkout; the shared
                # fixture must be restored to pristine so each leg exercises
                # the override from the pinned tree (verify-pin runs first).
                run("git", "-C", str(src), "checkout", "--", "go.mod", "go.sum")
                run("bash", str(self.helpers / "build-runner.sh"), str(src), str(out), asset, env=env)
                captured = json.loads((out / (asset + ".compiler-inputs.json")).read_text())
                self.assertIn("llama/llama.cpp/native.c", captured["files"])
                self.assertIn("llama/llama.cpp/vendor/nlohmann/json.hpp", captured["files"])
                self.assertIn("llama/llama.cpp/native.cpp", captured["files"])
                self.assertEqual(captured["dependency_files"], 2)
                data = json.loads((out / (asset + ".json")).read_text())
                if target == "windows-amd64":
                    dlls = (out / (asset + "-dlls.txt")).read_text()
                    self.assertIn("== ", dlls)
                    self.assertIn("KERNEL32.dll", dlls)
                else:
                    linkage = (out / (asset + "-linkage.txt")).read_text()
                    self.assertIn("libc.so.6" if target.startswith("linux-") else "/usr/lib/libSystem.B.dylib",
                                  linkage)
                raw = (out / data["executable"]).read_bytes()
                gz = (out / data["gzip"]).read_bytes()
                self.assertEqual(gzip.decompress(gz), raw)
                self.assertEqual(data["runner_sha256"], hashlib.sha256(raw).hexdigest())
                self.assertEqual(data["gzip_sha256"], hashlib.sha256(gz).hexdigest())
                self.assertEqual(gz[4:8], b'\0' * 4)
                self.assertEqual(gz[3], 0)  # no original filename
                run("python3", str(self.helpers / "package-runner.py"), "package",
                    str(out / data["executable"]), str(out / "again.gz"), str(out / "again.json"),
                    data["target"], data["upstream_commit"], "1.27.2")
                self.assertEqual(gz, (out / "again.gz").read_bytes())
        # The reviewed module bump applies in the build: the fixture pins
        # the x/* modules below the override, and every leg must have raised
        # each to exactly its declared MVS-selected version.
        gomod = (src / "go.mod").read_text()
        for spec in ("golang.org/x/crypto v0.57.0", "golang.org/x/image v0.47.0",
                     "golang.org/x/mod v0.41.0", "golang.org/x/net v0.59.0",
                     "golang.org/x/sync v0.24.0", "golang.org/x/sys v0.49.0",
                     "golang.org/x/term v0.46.0", "golang.org/x/text v0.43.0",
                     "golang.org/x/tools v0.51.0",
                     "github.com/buger/jsonparser v1.1.2"):
            self.assertIn(spec, gomod)
        self.assertNotIn("v0.22.0", gomod)
        self.assertNotIn("jsonparser v1.1.1", gomod)
        # The no-op path: a tree already carrying the fix exits 0 with no
        # changes (upstream caught up == retire the override). The helper
        # itself has no pin gate, so it is driven directly with the same
        # Go environment the build job exports.
        noop = self.root / "noop-src"
        noop.mkdir()
        (noop / "go.mod").write_text("module example.org/runner\n\ngo 1.26.0\n\nrequire golang.org/x/crypto v0.57.0\nrequire golang.org/x/image v0.47.0\nrequire golang.org/x/mod v0.41.0\nrequire golang.org/x/net v0.59.0\nrequire golang.org/x/sync v0.24.0\nrequire golang.org/x/sys v0.49.0\nrequire golang.org/x/term v0.46.0\nrequire golang.org/x/text v0.43.0\nrequire golang.org/x/tools v0.51.0\nrequire github.com/buger/jsonparser v1.1.2\n")
        go_env = dict(env, GOWORK="off", CGO_ENABLED="1", GOTOOLCHAIN="local")
        again = run("bash", str(self.helpers / "go-security-overrides.sh"), str(noop), env=go_env)
        self.assertIn("already satisfied", again.stdout + again.stderr)
        self.assertIn("golang.org/x/image v0.47.0", (noop / "go.mod").read_text())
        # Windows must never resolve a stock gcc/g++/clang: lab hosts and
        # minimal images have no GCC on PATH, and the verified MinGW clang
        # pair is the only compiler the recipe may use there. A PATH with
        # every other tool but no stock compiler must still build.
        lean = self.root / "lean-bin"
        lean.mkdir()
        for tool, target in [("go", mock / "go"), ("python3", Path(sys.executable)),
                             ("bash", Path(shutil.which("bash"))),
                             ("git", Path(shutil.which("git"))),
                             ("cygpath", mock / "cygpath")]:
            (lean / tool).symlink_to(target)
        # diff/sed are text tools, not compilers: the override gate needs
        # them the same way it needs grep/awk/sort, so they stay on PATH.
        for tool in ("mkdir", "mktemp", "rm", "grep", "awk", "sort", "cat", "dirname", "diff", "sed"):
            (lean / tool).symlink_to(shutil.which(tool))
        lean_env = dict(env, PATH=str(lean), TEST_HOST="windows/amd64")
        lean_out = self.root / "windows-lean"
        run("git", "-C", str(src), "checkout", "--", "go.mod", "go.sum")
        run(str(lean / "bash"), str(self.helpers / "build-runner.sh"),
            str(src), str(lean_out), "runner-windows-amd64", env=lean_env)
        self.assertTrue((lean_out / "runner-windows-amd64.exe").is_file())
        env["TEST_HOST"] = "linux/amd64"
        p = run("bash", str(self.helpers / "build-runner.sh"), str(src), str(self.root / "bad"),
                "runner-windows-amd64", env=env, ok=False)
        self.assertIn("native builder required", p.stderr)
        run("bash", str(self.helpers / "build-runner.sh"), str(src), str(self.root / "bad"),
            "runner-windows-arm64", env=env, ok=False)
    def test_capture_is_unique_under_parallel_compiles_and_keeps_compiler_argv(self):
        capture = self.root / "capture"
        compiler = self.root / "adversarial-cc"
        compiler.write_text("#!/usr/bin/env python3\nimport pathlib, sys, time\na=sys.argv[1:]\nassert a.count('-c') == 1\nsrc=next(x for x in a if x.endswith('.c'))\np=pathlib.Path(a[a.index('-MF')+1])\ntime.sleep(.03)\np.write_text('obj: ' + src + '\\n')\n")
        compiler.chmod(0o755)
        env = dict(os.environ, BASHY_REAL_CC=str(compiler), BASHY_COMPILER_CAPTURE_DIR=str(capture), BASHY_COMPILER_SOURCE=str(self.root))
        source = self.root / "one.c"
        source.write_text("int x;\n")
        command = [str(self.helpers / "compiler-capture-cc"), "-c", "-DKEEP=1", str(source)]
        procs = [subprocess.Popen(command, env=env, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True) for _ in range(12)]
        statuses = [p.communicate() + (p.returncode,) for p in procs]
        self.assertEqual([row[2] for row in statuses], [0] * len(procs), str(statuses))
        rows = [json.loads(p.read_text()) for p in (capture / "invocations").glob("*.json")]
        depfiles = list((capture / "depfiles").glob("*.d"))
        self.assertEqual(len(rows), 12)
        self.assertEqual(len({row["depfile"] for row in rows}), 12)
        self.assertEqual(len(depfiles), 12)
        self.assertTrue(all(row["captured"] and "-DKEEP=1" in row["argv"] for row in rows))

    def test_assemble_payload_prunes_directories_before_hashing(self):
        # Run 38555533555: the ollama audit artifacts contain directories
        # (notices/, runner-*.compiler-inputs/), and sha256sum -- * fails
        # on them. The shared template must prune dist/ subdirectories
        # after the missing-asset check and before hashing.
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        step = wf.split("assemble release payload", 1)[1].split("- name:", 1)[0]
        prune = "find dist -mindepth 1 -maxdepth 1 -type d -exec rm -rf {} +"
        self.assertIn(prune, step)
        self.assertIn("sha256sum", step)
        self.assertLess(step.index("missing"), step.index(prune))
        self.assertLess(step.index(prune), step.index("sha256sum"))


if __name__ == "__main__":
    unittest.main(verbosity=2)
