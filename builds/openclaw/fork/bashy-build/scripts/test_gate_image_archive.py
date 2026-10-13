#!/usr/bin/env python3
"""Tests for gate_image_archive.py + patch_rootfs.py.

Proves the review blocker stays fixed: a layered docker-save archive whose
UPPER layer carries the patched /app (so a merged-view /app-only gate
passes) still distributes the vulnerable copy in a LOWER layer, and the
archive gate fails it; the flattened single-layer archive passes.

The distributed asset is that archive gzip-compressed (*.docker.tar.gz —
the uncompressed save exceeds the 2 GiB release limit), so a second class
proves the gate opens the EXACT compressed bytes: vulnerable layers are
still caught through the compression, non-docker-save content is still
refused (never passed), corrupt input fails closed, and the --max-bytes
size gate fails oversized assets with a message.
"""

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


def gzip_bytes(raw):
    # mtime=0 like the release pipeline's `gzip -n`: deterministic bytes.
    return gzip.compress(raw, mtime=0)


def save_tgz(path, layers):
    """Write the docker-save-shaped tar from save_tar, then gzip it.

    Returns the raw (uncompressed) size; the file on disk is the exact
    distributed form: a gzip-compressed docker-save archive.
    """
    tmp = path + ".plain"
    save_tar(tmp, layers)
    with open(tmp, "rb") as f:
        raw = f.read()
    os.remove(tmp)
    with open(path, "wb") as f:
        f.write(gzip_bytes(raw))
    return len(raw)


class CompressedImageArchiveGate(unittest.TestCase):
    """The gate over the EXACT distributed bytes (*.docker.tar.gz)."""

    def setUp(self):
        self.d = tempfile.mkdtemp()

    def archive(self, name, layers):
        path = os.path.join(self.d, name)
        save_tgz(path, layers)
        return path

    def test_compressed_layered_archive_with_vulnerable_lower_layer_fails(self):
        # Same lower-layer leak as the plain-tar test, but through the
        # compression the release ships: the gate must still name the
        # lower layer, proving the scan covers the distributed content.
        lower = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.2.0")),
            (BASE_COPY + "/package.json", pkg_json("4.2.0")),
        ])
        upper = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.3.0")),
        ])
        path = self.archive("layered.docker.tar.gz", [lower, upper])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("below 4.3.0", r.stderr)
        self.assertIn("layer0", r.stderr)

    def test_compressed_fully_patched_archive_passes(self):
        layer = layer_bytes([
            (APP_COPY + "/package.json", pkg_json("4.3.0")),
            (BASE_COPY + "/package.json", pkg_json("4.3.0")),
        ])
        path = self.archive("flat.docker.tar.gz", [layer])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)
        self.assertIn("1 layer(s)", r.stdout)
        self.assertIn("2 copy/copies checked", r.stdout)

    def test_compressed_oci_layout_is_refused_not_passed(self):
        # Format refusal must survive compression: a gzipped OCI layout
        # is still an OCI layout, never a passing archive.
        plain = os.path.join(self.d, "oci.tar")
        with tarfile.open(plain, "w") as tf:
            for name in ("index.json", "oci-layout"):
                data = b"{}"
                ti = tarfile.TarInfo(name)
                ti.size = len(data)
                tf.addfile(ti, io.BytesIO(data))
        path = os.path.join(self.d, "oci.docker.tar.gz")
        with open(plain, "rb") as f:
            raw = f.read()
        os.remove(plain)
        with open(path, "wb") as f:
            f.write(gzip_bytes(raw))
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("OCI layout", r.stderr)

    def test_truncated_gzip_is_refused(self):
        # A truncated download must fail closed, never scan-or-pass.
        layer = layer_bytes([(APP_COPY + "/package.json", pkg_json("4.3.0"))])
        path = self.archive("cut.docker.tar.gz", [layer])
        with open(path, "rb") as f:
            raw = f.read()
        with open(path, "wb") as f:
            f.write(raw[:len(raw) // 2])
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("FAIL", r.stderr)

    def test_gzip_of_non_tar_is_refused(self):
        # Right extension, wrong content: gzip wrapping non-tar bytes
        # must be refused, never treated as a clean scan.
        path = os.path.join(self.d, "fake.docker.tar.gz")
        with open(path, "wb") as f:
            f.write(gzip_bytes(b"hello, not a tar archive"))
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("FAIL", r.stderr)

    def test_max_bytes_fails_oversized_clean_archive(self):
        # The size gate fires on byte size alone, even when the layer
        # scan is clean: an oversized asset fails here, not at upload.
        layer = layer_bytes([(APP_COPY + "/package.json", pkg_json("4.3.0"))])
        path = self.archive("big.docker.tar.gz", [layer])
        r = run(GATE, "--archive", path, "--max-bytes", "10")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("release limit", r.stderr)

    def test_max_bytes_boundary_is_fail_closed(self):
        # Releases refuse files >= 2 GiB, so size == limit fails and only
        # strictly-below passes.
        layer = layer_bytes([(APP_COPY + "/package.json", pkg_json("4.3.0"))])
        path = self.archive("edge.docker.tar.gz", [layer])
        size = os.path.getsize(path)
        r = run(GATE, "--archive", path, "--max-bytes", str(size))
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("release limit", r.stderr)
        r = run(GATE, "--archive", path, "--max-bytes", str(size + 1))
        self.assertEqual(r.returncode, 0, r.stdout + r.stderr)

    def test_negative_max_bytes_is_refused(self):
        layer = layer_bytes([(APP_COPY + "/package.json", pkg_json("4.3.0"))])
        path = self.archive("neg.docker.tar.gz", [layer])
        r = run(GATE, "--archive", path, "--max-bytes", "-1")
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("FAIL", r.stderr)

    def test_garbage_manifest_is_refused(self):
        # Right shape, unparseable manifest.json: refused with a message,
        # never a traceback pass or fail.
        layer = layer_bytes([(APP_COPY + "/package.json", pkg_json("4.3.0"))])
        plain = os.path.join(self.d, "garbage.tar")
        with tarfile.open(plain, "w") as tf:
            for name, data in [("manifest.json", b"not json{"),
                               ("config.json", b"{}"),
                               ("layer0/layer.tar", layer)]:
                ti = tarfile.TarInfo(name)
                ti.size = len(data)
                tf.addfile(ti, io.BytesIO(data))
        path = os.path.join(self.d, "garbage.docker.tar.gz")
        with open(plain, "rb") as f:
            raw = f.read()
        os.remove(plain)
        with open(path, "wb") as f:
            f.write(gzip_bytes(raw))
        r = run(GATE, "--archive", path)
        self.assertEqual(r.returncode, 1, r.stdout + r.stderr)
        self.assertIn("FAIL", r.stderr)
        self.assertIn("manifest.json", r.stderr)
        self.assertNotIn("Traceback", r.stdout + r.stderr)


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
