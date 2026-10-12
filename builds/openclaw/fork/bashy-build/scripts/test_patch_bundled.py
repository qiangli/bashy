#!/usr/bin/env python3
"""Tests for patch_bundled.py: GNU long paths, integrity, gate, SBOM."""

import base64
import gzip
import hashlib
import io
import json
import os
import subprocess
import sys
import tarfile
import tempfile
import unittest

HERE = os.path.dirname(os.path.abspath(__file__))
SCRIPT = os.path.join(HERE, "patch_bundled.py")
BUNDLED = "package/node_modules/npm/node_modules/http-cache-semantics"
LONG = "package/dist/" + "/".join(["very-long-directory-name-segment"] * 5) + "/file-with-a-long-name.js"


def add(tf, name, data=None, kind=tarfile.REGTYPE, mode=0o644, link=""):
    ti = tarfile.TarInfo(name)
    ti.type = kind
    ti.mode = mode
    ti.mtime = 499162500
    ti.linkname = link
    ti.size = len(data) if data is not None else 0
    tf.addfile(ti, io.BytesIO(data) if data is not None else None)


def make_tgz(path, entries, fmt=tarfile.GNU_FORMAT):
    with open(path, "wb") as raw:
        with gzip.GzipFile(filename="", mode="wb", fileobj=raw, mtime=0) as gz:
            with tarfile.open(fileobj=gz, mode="w", format=fmt) as tf:
                for e in entries:
                    add(tf, *e)


def run(*args):
    return subprocess.run([sys.executable, SCRIPT, *args], capture_output=True, text=True)


def members(path):
    with tarfile.open(path, "r:gz") as tf:
        return {m.name: (tf.extractfile(m).read() if m.isfile() else None) for m in tf.getmembers()}


class PatchBundled(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.tgz = os.path.join(self.d, "openclaw-gateway.tgz")
        make_tgz(self.tgz, [
            ("package/package.json", b'{"name":"openclaw","version":"2026.10.2"}'),
            (LONG, b"console.log('long path survives');\n"),
            ("package/node_modules/npm/node_modules", None, tarfile.DIRTYPE, 0o755),
            (BUNDLED, None, tarfile.DIRTYPE, 0o755),
            (BUNDLED + "/package.json", b'{"name":"http-cache-semantics","version":"4.2.0"}'),
            (BUNDLED + "/index.js", b"module.exports = 'old';\n"),
            ("package/bin/openclaw", b"#!/usr/bin/env node\n", tarfile.REGTYPE, 0o755),
            ("package/bin/oc", None, tarfile.SYMTYPE, 0o777, "openclaw"),
        ])
        self.repl = os.path.join(self.d, "http-cache-semantics-4.3.0.tgz")
        make_tgz(self.repl, [
            ("package/package.json", b'{"name":"http-cache-semantics","version":"4.3.0"}'),
            ("package/index.js", b"module.exports = 'fixed';\n"),
        ], fmt=tarfile.USTAR_FORMAT)
        raw = open(self.repl, "rb").read()
        self.integrity = "sha512-" + base64.b64encode(hashlib.sha512(raw).digest()).decode()
        self.sbom = os.path.join(self.d, "sbom.json")
        json.dump({"components": [
            {"name": "http-cache-semantics", "version": "4.2.0",
             "purl": "pkg:npm/http-cache-semantics@4.2.0", "bom-ref": "pkg:npm/http-cache-semantics@4.2.0"},
            {"name": "braces", "version": "3.0.3"}],
            "dependencies": [{"ref": "root", "dependsOn": ["pkg:npm/http-cache-semantics@4.2.0"]}]},
            open(self.sbom, "w"))

    def test_gate_fails_before_patch(self):
        r = run("--verify-only", "--tarball", self.tgz)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("below 4.3.0", r.stderr)

    def test_patch_preserves_long_paths_and_other_entries(self):
        before = members(self.tgz)
        r = run("--tarball", self.tgz, "--replacement", self.repl, "--integrity", self.integrity, "--sbom", self.sbom)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        after = members(self.tgz)
        self.assertIn(LONG, after)
        self.assertGreater(len(LONG), 100)
        for name, data in before.items():
            if not name.startswith(BUNDLED):
                self.assertEqual(after.get(name), data, name)
        self.assertIn(b'"4.3.0"', after[BUNDLED + "/package.json"])
        self.assertEqual(after[BUNDLED + "/index.js"], b"module.exports = 'fixed';\n")
        self.assertIn(BUNDLED, after)  # directory entry kept
        with tarfile.open(self.tgz, "r:gz") as tf:
            self.assertEqual(tf.getmember("package/bin/oc").linkname, "openclaw")
        self.assertEqual(run("--verify-only", "--tarball", self.tgz).returncode, 0)
        sb = json.load(open(self.sbom))
        hcs = sb["components"][0]
        self.assertEqual(hcs["version"], "4.3.0")
        self.assertEqual(hcs["purl"], "pkg:npm/http-cache-semantics@4.3.0")
        self.assertEqual(sb["dependencies"][0]["dependsOn"], ["pkg:npm/http-cache-semantics@4.3.0"])
        self.assertEqual(sb["components"][1]["version"], "3.0.3")

    def test_deterministic_output(self):
        other = os.path.join(self.d, "copy.tgz")
        open(other, "wb").write(open(self.tgz, "rb").read())
        for path in (self.tgz, other):
            r = run("--tarball", path, "--replacement", self.repl, "--integrity", self.integrity)
            self.assertEqual(r.returncode, 0, r.stderr)
        self.assertEqual(open(self.tgz, "rb").read(), open(other, "rb").read())

    def test_integrity_mismatch_refuses_and_leaves_tarball(self):
        original = open(self.tgz, "rb").read()
        bad = "sha512-" + base64.b64encode(hashlib.sha512(b"other").digest()).decode()
        r = run("--tarball", self.tgz, "--replacement", self.repl, "--integrity", bad)
        self.assertEqual(r.returncode, 1)
        self.assertIn("does not match the pinned sha512", r.stderr)
        self.assertEqual(open(self.tgz, "rb").read(), original)

    def test_nothing_to_patch_refuses(self):
        run("--tarball", self.tgz, "--replacement", self.repl, "--integrity", self.integrity)
        r = run("--tarball", self.tgz, "--replacement", self.repl, "--integrity", self.integrity)
        self.assertEqual(r.returncode, 1)
        self.assertIn("nothing to patch", r.stderr)


if __name__ == "__main__":
    unittest.main()
