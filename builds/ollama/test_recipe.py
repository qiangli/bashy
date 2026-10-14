#!/usr/bin/env python3
"""Bounded offline recipe gate. Fake compiler output is never native evidence."""
import gzip
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
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
        gate = (RECIPE / "files/windows-defender-egress.ps1").read_text()
        for marker in ("Defender control blocked", "pfirewall.log", "protocol gate PROTOCOL_PASS",
                       "no non-loopback connections observed",
                       "Defender block rules removed; firewall logging restored"):
            self.assertIn(marker, gate)

    def test_darwin_profile_uses_proven_loopback_form(self):
        # Prima-proven Seatbelt form (tag bashy-bashy-prima-0.1.2-13, run
        # 38486571631, green on the hosted macos-15 runners): (remote ip
        # "localhost:PORT"). The ip4 variant failed CI runs 38448516095 and
        # later. Reproduced on novidesign.local (macOS 26.3): the ip form
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


if __name__ == "__main__":
    unittest.main(verbosity=2)
