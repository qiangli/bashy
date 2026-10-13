#!/usr/bin/env python3
"""gate_image_archive.py — layer-wide vulnerable-copy gate for the distributed image archive.

The layered shipped image is FROM the unpatched base, so a vulnerable copy
survives in a LOWER layer even after /app is patched; `docker save`
distributes every layer. Scanning only the merged /app view is therefore
insufficient. This gate opens the distributed archive itself and FAILS if
ANY layer holds node_modules/<pkg>/package.json below --min-version —
anywhere in the filesystem, not just /app.

The distributed archive is the single-layer image's `docker save` output
piped through deterministic gzip (`gzip -n`), named *.docker.tar.gz: the
single-layer save of the full rootfs exceeds the 2 GiB release-file limit
uncompressed, so the compressed bytes ARE the release asset. This gate
opens the EXACT distributed file — transparently decompressing gzip/bz2/xz
(plain *.tar still accepted) — so the layer scan covers the exact
distributed content, and the digest/cosign signature bind the same
compressed bytes the gate checked.

It also asserts the archive IS a docker-save archive (manifest.json
present). An OCI layout (index.json / oci-layout) is refused with a message
saying exactly that, so a format change can never silently pass this gate.

The --max-bytes size gate (default 2 GiB) FAILS archives at or above the
limit, so an oversized asset fails here with a message instead of a late
release-upload error. --max-bytes 0 disables the size check.

Usage:
  gate_image_archive.py --archive IMAGE.docker.tar.gz [--package P]
      [--min-version V] [--max-bytes N]
"""

import argparse
import io
import json
import os
import re
import sys
import tarfile
import zlib

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from patch_bundled import below  # noqa: E402  (shared version comparison)


def die(msg):
    print(f"gate_image_archive: FAIL: {msg}", file=sys.stderr)
    sys.exit(1)


# Truncated or bit-corrupt compression surfaces as EOFError/zlib.error from
# inside tarfile/gzip, not TarError — every archive read below treats those
# as unreadable input and fails closed with a message, never a traceback.
READ_ERRORS = (tarfile.TarError, OSError, EOFError, zlib.error)


def pkg_json_re(pkg):
    return re.compile(r"(?:^|/)node_modules/" + re.escape(pkg) + r"/package\.json$")


def scan_layer(layer_f, pkg, minimum, layer_name):
    """[(path, version)] of every <pkg> copy in one layer tar stream."""
    rx = pkg_json_re(pkg)
    copies = []
    try:
        with tarfile.open(fileobj=layer_f, mode="r|*") as lt:
            for m in lt:
                if not m.isfile() or not rx.search(m.name):
                    continue
                data = lt.extractfile(m).read()
                try:
                    version = json.loads(data.decode("utf-8")).get("version")
                except (ValueError, UnicodeDecodeError):
                    version = None
                copies.append((f"{layer_name}:{m.name}", version))
    except READ_ERRORS as e:
        die(f"cannot read layer {layer_name}: {e}")
    return copies


RELEASE_LIMIT_BYTES = 2147483648  # 2 GiB: GitHub releases refuse files >= this


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--archive", required=True)
    p.add_argument("--package", default="http-cache-semantics")
    p.add_argument("--min-version", default="4.3.0")
    p.add_argument("--max-bytes", type=int, default=RELEASE_LIMIT_BYTES,
                   help="fail archives with byte size >= this (0 disables); "
                   f"default {RELEASE_LIMIT_BYTES} (2 GiB release limit)")
    args = p.parse_args(argv)

    # Fail-closed size gate over the EXACT distributed bytes (compressed
    # size, when the asset is compressed): an oversized asset must fail
    # here, never reach the release upload.
    if args.max_bytes < 0:
        die(f"--max-bytes {args.max_bytes} is negative (0 disables the size check)")
    if args.max_bytes > 0:
        try:
            size = os.path.getsize(args.archive)
        except OSError as e:
            die(f"cannot stat {args.archive} for the size gate: {e}")
        print(f"gate_image_archive: {args.archive}: {size} bytes "
              f"(limit: below {args.max_bytes})")
        if size >= args.max_bytes:
            die(f"{args.archive} is {size} bytes (>= {args.max_bytes} byte "
                f"release limit)")

    try:
        # "r:*": transparent compression (gzip/bz2/xz) over the exact
        # distributed file — the scan covers what ships, byte for byte.
        tf = tarfile.open(args.archive, "r:*")
    except READ_ERRORS as e:
        die(f"cannot open {args.archive} as a tar archive: {e}")
    with tf:
        try:
            names = set(tf.getnames())
        except READ_ERRORS as e:
            die(f"cannot list {args.archive}: {e}")
        if "manifest.json" not in names:
            if "index.json" in names or "oci-layout" in names:
                die(f"{args.archive} is an OCI layout, not a docker-save archive "
                    "(refusing: this gate covers docker-save layer tars)")
            die(f"{args.archive} has no manifest.json — not a docker-save image tar")
        try:
            manifest = json.load(tf.extractfile("manifest.json"))
        except (*READ_ERRORS, ValueError) as e:
            die(f"cannot parse manifest.json in {args.archive}: {e}")
        if not isinstance(manifest, list) or not manifest or \
                not isinstance(manifest[0], dict) or \
                not isinstance(manifest[0].get("Layers"), list):
            die("manifest.json has no Layers[]; refusing to guess its shape")
        layers = manifest[0]["Layers"]
        print(f"gate_image_archive: {args.archive}: docker-save archive, "
              f"{len(layers)} layer(s)")
        copies = []
        for layer_name in layers:
            try:
                m = tf.getmember(layer_name)
            except KeyError:
                die(f"manifest lists layer {layer_name}, missing from the archive")
            if not m.isfile():
                die(f"layer {layer_name} is not a regular file")
            copies.extend(scan_layer(tf.extractfile(m), args.package,
                                     args.min_version, layer_name))
    bad = [(path, v) for path, v in copies if below(v, args.min_version)]
    for path, v in sorted(copies):
        print(f"{path}: {args.package}@{v}")
    if bad:
        die(f"{len(bad)} copy/copies of {args.package} below {args.min_version} "
            f"in archive layers: " + ", ".join(f"{path}@{v}" for path, v in bad))
    print(f"gate_image_archive: OK — no {args.package} < {args.min_version} "
          f"in any of {len(layers)} layer(s) ({len(copies)} copy/copies checked)")


if __name__ == "__main__":
    main()
