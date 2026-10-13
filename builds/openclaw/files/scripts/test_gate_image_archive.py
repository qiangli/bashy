#!/usr/bin/env python3
"""Tests for gate_image_archive.py + patch_rootfs.py.

Proves the review blocker stays fixed: a layered docker-save archive whose
UPPER layer carries the patched /app (so a merged-view /app-only gate
passes) still distributes the vulnerable copy in a LOWER layer, and the
archive gate fails it; the flattened single-layer archive passes.
"""

import base64
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
GATE = os.path.join(HERE, "gate_image_archive.py")
PATCH_BUNDLED = os.path.join(HERE, "patch_bundled.py")
PATCH_ROOTFS = os.path.join(HERE, "patch_rootfs.py")

APP_COPY = "app/node_modules/npm/node_modules/http-cache-semantics"
BASE_COPY = "usr/local/lib/node_modules/npm/node_modules/http-cache-semantics"


def run(script, *args):
    return subprocess.run([sys.executable, script, *args],
                          capture_output=True, text=True)


def layer_bytes(entries):
    buf = io.BytesIO()
    with tarfile.open(fileobj=buf, mode="w") as tf:
        for name, data in entries:
            ti = tarfile.TarInfo(name)
            ti.type = tarfile.REGTYPE
            ti.mode = 0o644
            ti.mtime = 499162500
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))
    return buf.getvalue()


def pkg_json(version):
    return json.dumps({"name": "http-cache-semantics",
                       "version": version}).encode()


def save_tar(path, layers):
    """Write a docker-save-shaped tar: manifest.json + config + layer tars."""
    layer_names = [f"layer{i}/layer.tar" for i in range(len(layers))]
    manifest = json.dumps([{
        "Config": "config.json",
        "RepoTags": ["openclaw-gateway:test"],
        "Layers": layer_names,
    }]).encode()
    with tarfile.open(path, "w") as tf:
        for name, data in [("manifest.json", manifest),
                           ("config.json", b"{}")]:
            ti = tarfile.TarInfo(name)
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))
        for name, data in zip(layer_names, layers):
            ti = tarfile.TarInfo(name)
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))


def replacement_tgz(path):
    with tarfile.open(path, "w:gz") as tf:
        for name, data in [
                ("package/package.json", pkg_json("4.3.0")),
                ("package/index.js", b"module.exports = 'fixed';\n")]:
            ti = tarfile.TarInfo(name)
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))
    with open(path, "rb") as f:
        raw = f.read()
    return "sha512-" + base64.b64encode(hashlib.sha512(raw).digest()).decode()


class ImageArchiveGate(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()

    def archive(self, name, layers):
        path = os.path.join(self.d, name)
        save_tar(path, layers)
        return path

    def test_layered_archive_with_vulnerable_lower_layer_fails(self):
        # Lower layer: unpatched base (/app copy + npm copy outside /app).
        # Upper layer: patched /app only (what COPY package/ /app/ produces).
        # The merged view is clean, so the OLD /app-only gate passes, but the
        # distributed bytes still carry 4.2.0 — the archive gate must fail.
        lower = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.2.0")),
            (APP_COPY + "/index.js", b"old"),
            (BASE_COPY + "/package.json", pkg_json("4.2.0")),
            (BASE_COPY + "/index.js", b"old"),
        ])
        upper = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.3.0")),
            (APP_COPY + "/index.js", b"fixed"),
        ])
        path = self.archive("layered.tar", [lower, upper])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("below 4.3.0", r.stderr)
        self.assertIn("layer0", r.stderr)  # the lower layer is named

    def test_vulnerable_copy_outside_app_is_caught(self):
        # Even with /app fully patched in every layer, an npm-bundled copy
        # from the Node base outside /app fails the gate.
        layer = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.3.0")),
            (BASE_COPY + "/package.json", pkg_json("4.2.0")),
        ])
        path = self.archive("outside-app.tar", [layer])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn(BASE_COPY, r.stderr)

    def test_flattened_fully_patched_archive_passes(self):
        layer = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.3.0")),
            (BASE_COPY + "/package.json", pkg_json("4.3.0")),
        ])
        path = self.archive("flat.tar", [layer])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("1 layer(s)", r.stdout)
        self.assertIn("2 copy/copies checked", r.stdout)

    def test_oci_layout_is_refused_not_passed(self):
        path = os.path.join(self.d, "oci.tar")
        with tarfile.open(path, "w") as tf:
            for name in ("index.json", "oci-layout"):
                data = b"{}"
                ti = tarfile.TarInfo(name)
                ti.size = len(data)
                tf.addfile(ti, io.BytesIO(data))
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("OCI layout", r.stderr)

    def test_tar_without_manifest_is_refused(self):
        path = os.path.join(self.d, "plain.tar")
        with tarfile.open(path, "w") as tf:
            data = b"hi"
            ti = tarfile.TarInfo("hello.txt")
            ti.size = len(data)
            tf.addfile(ti, io.BytesIO(data))
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("no manifest.json", r.stderr)


class PatchRootfs(unittest.TestCase):
    def setUp(self):
        self.d = tempfile.mkdtemp()
        self.root = os.path.join(self.d, "rootfs")
        for copy in (APP_COPY, BASE_COPY):
            full = os.path.join(self.root, copy)
            os.makedirs(full)
            with open(os.path.join(full, "package.json"), "w") as f:
                json.dump({"name": "http-cache-semantics", "version": "4.2.0"}, f)
            with open(os.path.join(full, "index.js"), "w") as f:
                f.write("old")
        self.repl = os.path.join(self.d, "fixed.tgz")
        self.integrity = replacement_tgz(self.repl)

    def test_verify_only_fails_before_patch(self):
        r = run(PATCH_ROOTFS, "--verify-only", "--rootfs", self.root)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("below 4.3.0", r.stderr)

    def test_patch_replaces_every_copy_including_outside_app(self):
        r = run(PATCH_ROOTFS, "--rootfs", self.root,
                "--replacement", self.repl, "--integrity", self.integrity)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        for copy in (APP_COPY, BASE_COPY):
            full = os.path.join(self.root, copy)
            with open(os.path.join(full, "package.json")) as f:
                version = json.load(f)["version"]
            self.assertEqual(version, "4.3.0", copy)
            with open(os.path.join(full, "index.js")) as f:
                self.assertEqual(f.read(), "module.exports = 'fixed';\n", copy)
        self.assertEqual(
            run(PATCH_ROOTFS, "--verify-only", "--rootfs", self.root).returncode, 0)

    def test_patch_refuses_silent_noop(self):
        run(PATCH_ROOTFS, "--rootfs", self.root,
            "--replacement", self.repl, "--integrity", self.integrity)
        r = run(PATCH_ROOTFS, "--rootfs", self.root,
                "--replacement", self.repl, "--integrity", self.integrity)
        self.assertEqual(r.returncode, 1)
        self.assertIn("nothing to patch", r.stderr)

    def test_merged_view_gate_passes_while_lower_layer_still_vulnerable(self):
        # Documents WHY the archive gate exists: tar the merged rootfs view
        # (as the old image gate did via docker cp /app) and the tarball
        # gate passes — yet the layered archive above still fails.
        r = run(PATCH_ROOTFS, "--rootfs", self.root,
                "--replacement", self.repl, "--integrity", self.integrity)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        merged = os.path.join(self.d, "merged.tgz")
        with tarfile.open(merged, "w:gz") as tf:
            tf.add(os.path.join(self.root, "app"), arcname="package")
        r = run(PATCH_BUNDLED, "--verify-only", "--tarball", merged)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)


if __name__ == "__main__":
    unittest.main()
