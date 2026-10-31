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

# Whatever bash is on PATH (bashy on agent hosts). bashy runs its own cygpath
# applet ahead of PATH by design, so the mock is passed by path through
# BASHY_CYGPATH instead of relying on PATH order.
BASH = shutil.which("bash")

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
        base = {"source_commit": PIN, "toolkit_version": "12.8.1", "toolkit_components": [{"archive": "a.tar.xz", "sha256": "d" * 64}], "cuda_toolkit": "Build cuda_12.x", "nvcc_sha256": "a" * 64, "toolkit_sha256": "b" * 64,
                "preset": "CUDA 12", "artifacts": ["lib.so"]}
        ident.write_text(json.dumps(base))
        pkg = str(self.helpers / "package-runner.py")
        args = [pkg, "package", str(d / "runner"), str(d / "r.gz"), str(d / "runner-linux-amd64-cuda.json"),
                "linux/amd64", PIN, "1.27.3"]
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
        # cuda_fp16.h includes nv/target from CCCL, so the CUDA build must
        # pin that archive as a checked build-only input too.
        self.assertIn("cuda_cccl/linux-x86_64/cuda_cccl-linux-x86_64-12.8.90-archive.tar.xz=", env)
        install = self.helpers / "install-cuda-toolkit.sh"
        self.assertTrue(install.exists(), "pinned toolkit installer missing")
        text = install.read_text()
        self.assertIn("sha256sum", text)
        self.assertIn("CUDA_TOOLKIT_SHA256", text)
        self.assertIn('include/nv/target', text)
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("install-cuda-toolkit.sh", build)
        self.assertIn("CUDA_TOOLKIT_SHA256", build)
        self.assertIn("CUDA_USE_STATIC", build)  # static cudart/cublas link refused
        # Upstream ggml-cuda links libcuda (CUDA::cuda_driver) for its VMM pool;
        # NO_VMM keeps the driver reachable only through host cudart.
        configure = [l for l in build.splitlines() if '--preset \'CUDA 12\'' in l]
        self.assertEqual(len(configure), 1)
        self.assertIn("-DGGML_CUDA_NO_VMM=ON", configure[0])
        package = (self.helpers / "package-runner.py").read_text()
        self.assertIn("toolkit_sha256", package)
        self.assertIn("build-only", package)
        self.assertIn("host-provided", package)
        for banned in ("libcudart", "libcublas", "libcuda.so"):
            self.assertIn(banned, package)  # refused as backend artifacts

    def test_cuda_host_check_without_ldconfig_on_restricted_path(self):
        # The Linux no-egress gate runs under env -i PATH=/usr/bin:/bin, where
        # ldconfig (/usr/sbin) is not on PATH: no ldconfig means no host
        # runtime evidence, never a crash.
        tool = self.helpers / "cuda-link-audit.py"
        bare = self.root / "bare-path"
        bare.mkdir()
        env = {"PATH": str(bare)}
        cpu = subprocess.run([sys.executable, str(tool), "host", "--cpu-explicit"],
                             env=env, capture_output=True, text=True)
        self.assertEqual(cpu.returncode, 0, cpu.stdout + cpu.stderr)
        self.assertIn("host lacks", cpu.stdout)
        strict = subprocess.run([sys.executable, str(tool), "host"],
                                env=env, capture_output=True, text=True)
        self.assertNotEqual(strict.returncode, 0)
        self.assertIn("ldconfig", strict.stdout + strict.stderr)
        gate = (self.helpers / "native-gate.py").read_text()
        record = gate.index("'cpu_explicit_returncode'")
        self.assertLess(record, gate.index("require(cpu.returncode == 0"))

    def test_cuda_link_audit_and_host_runtime_fail_closed(self):
        # S451.4a review: the backend must link NVIDIA libs dynamically only
        # (NEEDED, no defined cuda*/cublas* symbols), and a host without the
        # runtime must fail closed; CPU is used only when explicitly requested.
        tool = self.helpers / "cuda-link-audit.py"
        self.assertTrue(tool.exists(), "cuda-link-audit.py missing")
        d = self.root / "link"
        d.mkdir()
        good = d / "good.txt"
        good.write_text(" 0x1 (NEEDED) Shared library: [libcudart.so.12]\n 0x1 (NEEDED) Shared library: [libcublas.so.12]\n"
                        " 0x1 (NEEDED) Shared library: [libc.so.6]\n\n    10: 0 FUNC GLOBAL DEFAULT UND cudaMalloc\n")
        run("python3", str(tool), "link", "--readelf-file", str(good))
        static = d / "static.txt"
        static.write_text(" 0x1 (NEEDED) Shared library: [libcudart.so.12]\n\n    10: 1000 FUNC GLOBAL DEFAULT 12 cublasCreate_v2\n")
        run("python3", str(tool), "link", "--readelf-file", str(static), ok=False)
        nodyn = d / "nodyn.txt"
        nodyn.write_text(" 0x1 (NEEDED) Shared library: [libc.so.6]\n")
        run("python3", str(tool), "link", "--readelf-file", str(nodyn), ok=False)
        bundled = d / "bundled.txt"
        bundled.write_text(good.read_text().replace("libc.so.6", "libcuda.so.1") + " 0x1 (RPATH) Library rpath: [$ORIGIN]\n")
        ok = run("python3", str(tool), "link", "--readelf-file", str(bundled), ok=False)
        self.assertIn("libcuda", ok.stdout + ok.stderr)
        empty = d / "ldconfig-empty.txt"
        empty.write_text("libc.so.6 (libc6,x86-64) => /lib/x86_64-linux-gnu/libc.so.6\n")
        refuse = run("python3", str(tool), "host", "--ldconfig-file", str(empty), ok=False)
        self.assertIn("fails closed", refuse.stdout + refuse.stderr)
        cpu = run("python3", str(tool), "host", "--ldconfig-file", str(empty), "--cpu-explicit")
        self.assertIn("cpu-explicit", cpu.stdout)
        full = d / "ldconfig-full.txt"
        full.write_text("libcuda.so.1 => /usr/lib/libcuda.so.1\nlibcudart.so.12 => /x/libcudart.so.12\nlibcublas.so.12 => /x/libcublas.so.12\n")
        self.assertIn("cuda", run("python3", str(tool), "host", "--ldconfig-file", str(full)).stdout)
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("cuda-link-audit.py\" link", build)

    def test_cuda_audit_release_and_sbom_cover_cuda_target(self):
        # S451 review: the generated workflow, audit runner and release list
        # must treat the CUDA target as a sixth native record; its toolkit is
        # an SBOM component; transitive backend libraries are enumerated; and
        # the host-runtime fail-closed check is wired into the native gate.
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        self.assertNotIn("all five native", wf)
        self.assertIn("download all six native build records", wf)
        self.assertRegex(wf, r"for target in linux-amd64 linux-arm64 darwin-amd64 darwin-arm64 windows-amd64 linux-amd64-cuda; do\s+binary=")
        text = (self.helpers / "audit-runner.py").read_text()
        self.assertIn('CUDA_TARGET = "linux-amd64-cuda"', text)
        self.assertIn("ALL_TARGETS = TARGETS + (CUDA_TARGET,)", text)
        env = (RECIPE / "tool.env").read_text()
        assets = re.search(r'^RELEASE_ASSETS="([^"]*)"', env, re.M).group(1).split()
        for suffix in ("-native.json", "-native.log", "-egress.txt", "-cuda-linkage.txt", ".backend-artifacts.json"):
            self.assertIn("runner-linux-amd64-cuda" + suffix, assets)
        sbom = (self.helpers / "audit-runner.py").read_text()
        self.assertIn("build-only-sdk", sbom)
        native = (self.helpers / "native-gate.py").read_text()
        self.assertIn("cuda-link-audit.py", native)
        self.assertIn("host_runtime", native)
        tool = self.helpers / "cuda-link-audit.py"
        d = self.root / "needed"
        d.mkdir()
        f = d / "r.txt"
        f.write_text(" 0x1 (NEEDED) Shared library: [libcudart.so.12]\n 0x1 (NEEDED) Shared library: [libggml-base.so]\n"
                     " 0x1 (NEEDED) Shared library: [libc.so.6]\n 0x1 (NEEDED) Shared library: [libstdc++.so.6]\n")
        got = run("python3", str(tool), "needed", "--readelf-file", str(f)).stdout.split()
        self.assertEqual(got, ["libggml-base.so"])
        self.assertIn("needed", (self.helpers / "build-runner.sh").read_text())

    def test_cuda_manifest_records_toolkit_components(self):
        d = self.root / "tk"
        d.mkdir()
        (d / "runner").write_bytes(b"exe")
        (d / "lib.so").write_bytes(b"backend")
        comps = [{"archive": "cuda_nvcc-12.8.93.tar.xz", "sha256": "c" * 64}]
        ident = d / "id.json"
        ident.write_text(json.dumps({"source_commit": PIN, "cuda_toolkit": "Build cuda_12.8", "nvcc_sha256": "a" * 64,
                                     "toolkit_sha256": "b" * 64, "toolkit_version": "12.8.1", "toolkit_components": comps,
                                     "preset": "CUDA 12", "artifacts": ["lib.so"]}))
        run("python3", str(self.helpers / "package-runner.py"), "package", str(d / "runner"), str(d / "r.gz"),
            str(d / "runner-linux-amd64-cuda.json"), "linux/amd64", PIN, "1.27.3", str(ident))
        m = json.loads((d / "runner-linux-amd64-cuda.json").read_text())
        self.assertEqual(m["backend_identity"]["toolkit_components"], comps)
        self.assertEqual(m["backend_identity"]["toolkit_version"], "12.8.1")
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("toolkit_components", build)

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
 print({'GOHOSTOS':os.environ['TEST_HOST'].split('/')[0], 'GOHOSTARCH':os.environ['TEST_HOST'].split('/')[1], 'GOVERSION':'go1.27.3', 'GOROOT':str(pathlib.Path.cwd() / 'goroot')}[args[1]])
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
                   BASHY_WINDOWS_OBJDUMP=str(mock / 'llvm-objdump'),
                   BASHY_CYGPATH=str(mock / 'cygpath'))
        for target in TARGETS:
            with self.subTest(target=target):
                asset = "runner-" + target
                env["TEST_HOST"] = target.replace("-", "/")
                out = self.root / target
                # CI gives every matrix leg a fresh checkout; the shared
                # fixture must be restored to pristine so each leg exercises
                # the override from the pinned tree (verify-pin runs first).
                run("git", "-C", str(src), "checkout", "--", "go.mod", "go.sum")
                run(BASH, str(self.helpers / "build-runner.sh"), str(src), str(out), asset, env=env)
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
                    data["target"], data["upstream_commit"], "1.27.3")
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
                             ("bash", Path(BASH)),
                             
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
        p = run(BASH, str(self.helpers / "build-runner.sh"), str(src), str(self.root / "bad"),
                "runner-windows-amd64", env=env, ok=False)
        self.assertIn("native builder required", p.stderr)
        run(BASH, str(self.helpers / "build-runner.sh"), str(src), str(self.root / "bad"),
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

    def test_nonpublishing_candidate_dispatch_builds_and_audits_cuda(self):
        # S451: the CUDA target needs native build/audit evidence before any
        # release tag. ollama gets a workflow_dispatch candidate path that
        # runs pin/build/audit but can never sign, attest or publish.
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        head = wf.split("\njobs:", 1)[0]
        self.assertIn("workflow_dispatch:", head)
        self.assertIn("tags:", head)
        release = wf.split("  release:\n", 1)[1]
        cond = release.split("runs-on:", 1)[0]
        self.assertIn("github.ref_type == 'tag'", cond)
        self.assertNotIn("workflow_dispatch", release)
        pin = wf.split("  pin:\n", 1)[1].split("\n  build:", 1)[0]
        self.assertIn("candidate", pin)
        self.assertIn("linux-amd64-cuda", wf)

    def test_release_assets_are_derived_from_backend_manifest_fail_closed(self):
        # Review 2026-10-20: every backend artifact in the manifest (incl.
        # transitive ggml libs) must be present, hash-verified, and therefore
        # published and signed; an unlisted shared library must not slip in.
        tool = self.helpers / "release-assets-check.py"
        self.assertTrue(tool.exists(), "release-assets-check.py missing")
        d = Path(tempfile.mkdtemp())
        (d / "libggml-cuda.so").write_bytes(b"cuda")
        (d / "libggml-base.so").write_bytes(b"base")
        (d / "runner.json").write_text(json.dumps({"backend_artifacts": [
            {"name": "libggml-cuda.so", "sha256": hashlib.sha256(b"cuda").hexdigest()},
            {"name": "libggml-base.so", "sha256": hashlib.sha256(b"base").hexdigest()}]}))
        run = lambda *a: subprocess.run(["python3", str(tool), *a], capture_output=True, text=True)
        ok = run(str(d), "libggml-cuda.so runner.json libggml-base.so")
        self.assertEqual(ok.returncode, 0, ok.stderr)
        self.assertIn("libggml-base.so", ok.stdout)
        # manifest-derived artifact absent from the static list is still required
        (d / "libggml-base.so").unlink()
        self.assertNotEqual(run(str(d), "libggml-cuda.so runner.json").returncode, 0)
        (d / "libggml-base.so").write_bytes(b"tampered")
        self.assertNotEqual(run(str(d), "libggml-cuda.so runner.json").returncode, 0)
        (d / "libggml-base.so").write_bytes(b"base")
        (d / "libstray.so.1").write_bytes(b"x")
        self.assertNotEqual(run(str(d), "libggml-cuda.so runner.json").returncode, 0)
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        step = wf.split("assemble release payload", 1)[1].split("- name:", 1)[0]
        self.assertIn("release-assets-check.py", step)

    def test_cuda_cu_inputs_enter_compiler_capture(self):
        # Review 2026-10-20: nvcc/.cu compile inputs must be captured through a
        # CMake compiler launcher and reach the compiler-input review.
        launcher = self.helpers / "compiler-capture-launcher"
        self.assertTrue(launcher.exists(), "compiler-capture-launcher missing")
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("CMAKE_CUDA_COMPILER_LAUNCHER", build)
        self.assertIn("compiler-capture-launcher", build)
        self.assertIn("--generated", build)
        d = Path(tempfile.mkdtemp())
        src = d / "src"; src.mkdir()
        (src / "k.cu").write_text("__global__ void k(){}\n")
        fake = d / "fakenvcc"
        fake.write_text("#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -MF ] && { shift; echo \"o: $SRCFILE\" > \"$1\"; }; shift; done\n")
        fake.chmod(0o755)
        cap = d / "cap"
        env = dict(os.environ, BASHY_COMPILER_CAPTURE_DIR=str(cap), BASHY_COMPILER_SOURCE=str(src),
                   BASHY_COMPILER_GOROOT=str(d), BASHY_COMPILER_TMP=str(d), SRCFILE=str(src / "k.cu"))
        r = subprocess.run([str(launcher), str(fake), "-c", str(src / "k.cu"), "-MD", "-MF", str(d / "cmake.d")],
                           env=env, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        recs = [json.loads(f.read_text()) for f in (cap / "invocations").glob("*.json")]
        self.assertEqual(len(recs), 1)
        self.assertTrue(recs[0]["captured"])
        self.assertTrue((d / "cmake.d").is_file(), "CMake's own depfile must be preserved")
        self.assertEqual([i["relative"] for i in recs[0]["inputs"]], ["k.cu"])

    def test_relative_deleted_depfile_survives_to_compiler_inputs_audit(self):
        # Hosted run 41168017074: CMake passes a cwd-relative -MF and ninja
        # consumes/deletes the .d after the compile; the launcher must snapshot
        # it into the capture dir so audit-runner (another cwd, later) finds it.
        launcher = self.helpers / "compiler-capture-launcher"
        d = Path(tempfile.mkdtemp())
        src = d / "src"; (src / "build").mkdir(parents=True)
        (src / "k.cu").write_text("__global__ void k(){}\n")
        fake = d / "fakenvcc"
        fake.write_text("#!/bin/sh\nwhile [ $# -gt 0 ]; do [ \"$1\" = -MF ] && { shift; echo \"o: ../k.cu\" > \"$1\"; }; shift; done\n")
        fake.chmod(0o755)
        cap = d / "cap"
        env = dict(os.environ, BASHY_COMPILER_CAPTURE_DIR=str(cap), BASHY_COMPILER_SOURCE=str(src),
                   BASHY_COMPILER_GOROOT=str(d), BASHY_COMPILER_TMP=str(d))
        r = subprocess.run([str(launcher), str(fake), "-c", "../k.cu", "-MD", "-MF", "k.cu.o.d"],
                           env=env, capture_output=True, text=True, cwd=src / "build")
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertTrue((src / "build" / "k.cu.o.d").is_file(), "CMake's relative depfile must stay in place")
        (src / "build" / "k.cu.o.d").unlink()
        rec = json.loads(next((cap / "invocations").glob("*.json")).read_text())
        self.assertTrue(os.path.isabs(rec["depfile"]))
        self.assertTrue(rec["depfile"].startswith(str(cap / "depfiles")), rec["depfile"])
        self.assertEqual(rec["source_depfile"], "k.cu.o.d")
        out = d / "inputs.json"
        r = subprocess.run(["python3", str(self.helpers / "audit-runner.py"), "compiler-inputs", "--capture", str(cap),
                            "--source", str(src), "--generated", str(d / "none"), "--output", str(out)],
                           capture_output=True, text=True, cwd=d)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        data = json.loads(out.read_text())
        self.assertEqual(sorted(data["files"]), ["k.cu"])
        self.assertEqual(data["dependency_files"], 1)

    def test_backend_dependency_closure_is_recursive_and_fail_closed(self):
        tool = self.helpers / "cuda-link-audit.py"
        d = Path(tempfile.mkdtemp())
        tree = d / "tree"; tree.mkdir()
        for n in ("libggml-base.so", "libggml-cpu.so", "libggml-cuda.so"):
            (tree / n).write_bytes(b"x")
        fake = d / "readelf"
        fake.write_text("#!/bin/sh\nfor a; do f=$a; done\ncase $(basename $f) in\n"
            "libggml-cuda.so) echo ' 0x1 (NEEDED) Shared library: [libggml-base.so]'; echo ' 0x1 (NEEDED) Shared library: [libcudart.so.12]';;\n"
            "libggml-base.so) echo ' 0x1 (NEEDED) Shared library: [libggml-cpu.so]'; echo ' 0x1 (NEEDED) Shared library: [libc.so.6]';;\n"
            "libggml-cpu.so) echo ' 0x1 (NEEDED) Shared library: [libm.so.6]';;\n"
            "esac\n")
        fake.chmod(0o755)
        run_ = lambda: subprocess.run(["python3", str(tool), "closure", str(tree / "libggml-cuda.so"), "--search", str(tree), "--readelf", str(fake)],
                                      capture_output=True, text=True)
        r = run_()
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(sorted(r.stdout.split()), ["libggml-base.so", "libggml-cpu.so"])
        (tree / "libggml-cpu.so").unlink()
        self.assertNotEqual(run_().returncode, 0, "unresolved nested dep must fail")
        (tree / "libggml-cpu.so").write_bytes(b"x")
        fake.write_text(fake.read_text().replace("[libm.so.6]", "[libcuda.so.1]"))
        self.assertNotEqual(run_().returncode, 0, "nested host-driver dep must fail")
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn('cuda-link-audit.py" closure', build)

    def test_soname_symlink_dependency_resolves_to_real_file(self):
        tool = self.helpers / "cuda-link-audit.py"
        d = Path(tempfile.mkdtemp())
        tree = d / "tree" / "lib"; tree.mkdir(parents=True)
        (tree / "libggml-cuda.so").write_bytes(b"x")
        (tree / "libggml-base.so.0.0.0").write_bytes(b"real")
        (tree / "libggml-base.so.0").symlink_to("libggml-base.so.0.0.0")
        fake = d / "readelf"
        fake.write_text("#!/bin/sh\nfor a; do f=$a; done\ncase $(basename $f) in\n"
            "libggml-cuda.so) echo ' 0x1 (NEEDED) Shared library: [libggml-base.so.0]';;\n"
            "libggml-base.so.0.0.0) echo ' 0x1 (NEEDED) Shared library: [libc.so.6]';;\n"
            "esac\n")
        fake.chmod(0o755)
        args = ["python3", str(tool), "closure", str(tree / "libggml-cuda.so"), "--search", str(d / "tree"), "--readelf", str(fake)]
        r = subprocess.run(args, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.split(), ["libggml-base.so.0"])
        outside = d / "outside.so"; outside.write_bytes(b"x")
        (tree / "libggml-base.so.0").unlink()
        (tree / "libggml-base.so.0").symlink_to(outside)
        self.assertNotEqual(subprocess.run(args, capture_output=True, text=True).returncode, 0, "symlink escaping tree must fail")

    def test_build_runner_packages_soname_symlink_under_soname(self):
        build = (self.helpers / "build-runner.sh").read_text()
        a = build.index("  extra=()")
        b = build.index("  python3 - \"$out/$asset.backend-artifacts.json\"")
        seg = build[a:b]
        d = Path(tempfile.mkdtemp())
        (d / "cuda-link-audit.py").write_text("print('libggml-base.so.0')\n")
        (d / "out").mkdir()
        script = ("set -euo pipefail\nhere=%s out=%s asset=a BASHY_COMPILER_TMP=%s\nmkdir -p \"$BASHY_COMPILER_TMP/cuda-build/lib\"\n"
                  "printf real >\"$BASHY_COMPILER_TMP/cuda-build/lib/libggml-base.so.0.0.0\"\n"
                  "ln -s libggml-base.so.0.0.0 \"$BASHY_COMPILER_TMP/cuda-build/lib/libggml-base.so.0\"\n"
                  "readelf() { :; }\ntouch \"$out/a-libggml-cuda.so\"\n%s\necho PACKAGED \"${extra[*]}\"\n"
                  % (d, d / "out", d / "tmp", seg))
        r = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        got = d / "out" / "a-libggml-base.so.0"
        self.assertTrue(got.is_file() and not got.is_symlink())
        self.assertEqual(got.read_bytes(), b"real")
        self.assertIn("a-libggml-base.so.0", r.stdout)

    def test_closure_ships_only_ggml_and_reports_nvidia_host_required(self):
        tool = self.helpers / "cuda-link-audit.py"
        d = Path(tempfile.mkdtemp())
        tree = d / "tree"; (tree / "lib").mkdir(parents=True); (tree / "toolkit" / "lib64").mkdir(parents=True)
        (tree / "lib" / "libggml-cuda.so").write_bytes(b"x")
        (tree / "lib" / "libggml-base.so.0.0.0").write_bytes(b"real")
        (tree / "lib" / "libggml-base.so.0").symlink_to("libggml-base.so.0.0.0")
        (tree / "toolkit" / "lib64" / "libcudart.so.12.8.90").write_bytes(b"nv")
        (tree / "toolkit" / "lib64" / "libcudart.so.12").symlink_to("libcudart.so.12.8.90")
        fake = d / "readelf"
        fake.write_text("#!/bin/sh\nfor a; do f=$a; done\ncase $(basename $f) in\n"
            "libggml-cuda.so) echo ' 0x1 (NEEDED) Shared library: [libcudart.so.12]'; echo ' 0x1 (NEEDED) Shared library: [libcublasLt.so.12]'; echo ' 0x1 (NEEDED) Shared library: [libggml-base.so.0]';;\n"
            "libggml-base.so.0.0.0) echo ' 0x1 (NEEDED) Shared library: [libc.so.6]';;\n"
            "esac\n")
        fake.chmod(0o755)
        hr = d / "host-required.txt"
        args = ["python3", str(tool), "closure", str(tree / "lib" / "libggml-cuda.so"), "--search", str(tree), "--readelf", str(fake),
                "--host-required-out", str(hr)]
        r = subprocess.run(args, capture_output=True, text=True)
        self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(r.stdout.split(), ["libggml-base.so.0"])
        self.assertEqual(sorted(hr.read_text().split()), ["libcublasLt.so.12", "libcudart.so.12"])
        (tree / "lib" / "libfoo.so.1").write_bytes(b"x")
        fake.write_text(fake.read_text().replace("[libc.so.6]", "[libfoo.so.1]"))
        r = subprocess.run(args, capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("libfoo.so.1", r.stderr)
        build = (self.helpers / "build-runner.sh").read_text()
        self.assertIn("--host-required-out", build)
        self.assertIn("host-required", build)

    def test_build_runner_refuses_non_ggml_packaging(self):
        build = (self.helpers / "build-runner.sh").read_text()
        a = build.index("  extra=()")
        b = build.index("  python3 - \"$out/$asset.backend-artifacts.json\"")
        seg = build[a:b]
        d = Path(tempfile.mkdtemp())
        (d / "cuda-link-audit.py").write_text("print('libcudart.so.12')\n")
        (d / "out").mkdir()
        script = ("set -euo pipefail\nhere=%s out=%s asset=a BASHY_COMPILER_TMP=%s\nmkdir -p \"$BASHY_COMPILER_TMP/cuda-build/lib\"\n"
                  "printf nv >\"$BASHY_COMPILER_TMP/cuda-build/lib/libcudart.so.12\"\n"
                  "readelf() { :; }\ntouch \"$out/a-libggml-cuda.so\"\n%s\necho PACKAGED\n" % (d, d / "out", d / "tmp", seg))
        r = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("PACKAGED", r.stdout)
        self.assertIn("libcudart.so.12", r.stderr)
        self.assertFalse((d / "out" / "a-libcudart.so.12").exists())

    def test_build_runner_aborts_when_closure_helper_fails(self):
        build = (self.helpers / "build-runner.sh").read_text()
        a = build.index("  extra=()")
        b = build.index("  python3 - \"$out/$asset.backend-artifacts.json\"")
        seg = build[a:b]
        d = Path(tempfile.mkdtemp())
        (d / "cuda-link-audit.py").write_text("import sys\nprint('libfake.so')\nsys.exit(3)\n")
        (d / "tree").mkdir(); (d / "tree" / "libfake.so").write_bytes(b"x")
        (d / "out").mkdir()
        script = ("set -euo pipefail\nhere=%s out=%s asset=a BASHY_COMPILER_TMP=%s\nmkdir -p \"$BASHY_COMPILER_TMP/cuda-build\"\n"
                  "cp %s \"$BASHY_COMPILER_TMP/cuda-build/libfake.so\"\nreadelf() { :; }\ntouch \"$out/a-libggml-cuda.so\"\n%s\necho PACKAGED\n"
                  % (d, d / "out", d / "tmp", d / "tree" / "libfake.so", seg))
        r = subprocess.run(["bash", "-c", script], capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertNotIn("PACKAGED", r.stdout)

    def test_runner_toolchain_carries_stdlib_x509_fix(self):
        def ver(v):
            return tuple(int(x) for x in v.split("."))
        env = dict(l.split("=", 1) for l in (RECIPE / "tool.env").read_text().splitlines()
                   if re.match(r"^GO_TOOLCHAIN(_SECURITY_OVERRIDE)?=", l))
        effective = env.get("GO_TOOLCHAIN_SECURITY_OVERRIDE", env["GO_TOOLCHAIN"])
        self.assertGreaterEqual(ver(effective), (1, 27, 3), "GO-2026-6809 is fixed in go1.27.3")
        for f in ("setup.yml", "audit.yml"):
            m = re.search(r"go-version: '([^']+)'", (RECIPE / f).read_text())
            self.assertTrue(m.group(1) == "@@GO_TOOLCHAIN@@" or ver(m.group(1)) >= (1, 27, 3), f)
        pin = (self.helpers / "PIN").read_text()
        self.assertIn("GO_TOOLCHAIN=" + env["GO_TOOLCHAIN"], pin)

    def test_prerelease_is_per_tool_and_cuda_libs_are_release_assets(self):
        wf = (self.fork / ".github/workflows/bashy-build.yml").read_text()
        create = wf.split("gh release create", 1)[1]
        self.assertIn("--prerelease", create)
        other = self.root / "other"
        run("bash", str(ROOT / "builds/render.sh"), "zot", str(other))
        owf = (other / ".github/workflows/bashy-build.yml").read_text()
        self.assertNotIn("--prerelease", owf)
        env = dict(l.split("=", 1) for l in (RECIPE / "tool.env").read_text().splitlines()
                   if re.match(r"^(RELEASE_ASSETS|RELEASE_PRERELEASE)=", l))
        self.assertEqual(env.get("RELEASE_PRERELEASE"), "1")
        assets = env["RELEASE_ASSETS"].strip('"').split()
        packaged = {"runner-linux-amd64-cuda-libggml-cuda.so", "runner-linux-amd64-cuda-libggml-base.so.0"}
        for name in packaged:
            self.assertIn(name, assets)
        # the check refuses a packaged backend file the static release list omits
        tool = self.helpers / "release-assets-check.py"
        d = Path(tempfile.mkdtemp())
        (d / "libggml-cuda.so").write_bytes(b"c")
        (d / "libggml-base.so.0").write_bytes(b"b")
        (d / "runner.json").write_text(json.dumps({"backend_artifacts": [
            {"name": "libggml-cuda.so", "sha256": hashlib.sha256(b"c").hexdigest()},
            {"name": "libggml-base.so.0", "sha256": hashlib.sha256(b"b").hexdigest()}]}))
        r = subprocess.run(["python3", str(tool), str(d), "libggml-cuda.so runner.json"], capture_output=True, text=True)
        self.assertNotEqual(r.returncode, 0)
        self.assertIn("libggml-base.so.0", r.stderr)

    def test_link_audit_rejects_absolute_runpath_and_configure_sets_origin(self):
        tool = self.helpers / "cuda-link-audit.py"
        d = self.root / "runpath"
        d.mkdir()
        base = (" 0x1 (NEEDED) Shared library: [libcudart.so.12]\n 0x1 (NEEDED) Shared library: [libcublas.so.12]\n"
                " 0x1 (NEEDED) Shared library: [libggml-base.so.0]\n")
        cases = {
            "none": ("", True),
            "origin": (" 0x1d (RUNPATH) Library runpath: [$ORIGIN]\n", True),
            "tmp": (" 0x1d (RUNPATH) Library runpath: [/tmp/tmp.lfg7yKmpJl/cuda-build/lib/ollama:]\n", False),
            "rpath": (" 0x0f (RPATH) Library rpath: [/opt/build/lib]\n", False),
            "mixed": (" 0x1d (RUNPATH) Library runpath: [$ORIGIN:/tmp/x]\n", False),
            "origin_trailing_colon": (" 0x1d (RUNPATH) Library runpath: [$ORIGIN:]\n", True),
            "origin_twice": (" 0x1d (RUNPATH) Library runpath: [$ORIGIN:$ORIGIN]\n", True),
            "empty": (" 0x1d (RUNPATH) Library runpath: []\n", False),
        }
        for name, (extra, ok) in cases.items():
            f = d / f"{name}.txt"
            f.write_text(base + extra)
            r = run("python3", str(tool), "link", "--readelf-file", str(f), ok=ok)
            if not ok:
                self.assertIn("PATH must be absent", r.stdout + r.stderr.replace("RUNPATH", "PATH").replace("RPATH", "PATH"), name)
        build = (self.helpers / "build-runner.sh").read_text()
        configure = [l for l in build.splitlines() if "--preset 'CUDA 12'" in l]
        self.assertEqual(len(configure), 1)
        self.assertIn("-DCMAKE_BUILD_RPATH='$ORIGIN'", configure[0])
        self.assertIn("-DCMAKE_BUILD_RPATH_USE_ORIGIN=ON", configure[0])

    def test_release_note_states_side_by_side_install_and_silent_cpu_fallback(self):
        env = (RECIPE / "tool.env").read_text()
        note = re.search(r'RELEASE_NOTE="([^"]*)"', env).group(1)
        for needle in ("side by side", "$ORIGIN", "libcudart", "libcublas", "silently falls back to CPU", "fails closed"):
            self.assertIn(needle, note)

    def test_release_note_and_gates_describe_cuda_six_targets(self):
        env = (RECIPE / "tool.env").read_text()
        note = re.search(r'RELEASE_NOTE="([^"]*)"', env).group(1)
        gates = re.search(r'RELEASE_GATES="([^"]*)"', env).group(1)
        self.assertNotIn("No bundled CUDA", note)
        self.assertIn("ggml CUDA backend", note)
        self.assertIn("host-provided", note)
        self.assertNotIn("five native", gates)
        self.assertIn("six native", gates)


if __name__ == "__main__":
    unittest.main(verbosity=2)
