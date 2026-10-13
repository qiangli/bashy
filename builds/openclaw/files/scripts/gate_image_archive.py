#!/usr/bin/env python3
"""gate_image_archive.py — layer-wide vulnerable-copy gate for the distributed image tar.

The layered shipped image is FROM the unpatched base, so a vulnerable copy
survives in a LOWER layer even after /app is patched; `docker save`
distributes every layer. Scanning only the merged /app view is therefore
insufficient. This gate opens the saved archive itself and FAILS if ANY layer
holds node_modules/<pkg>/package.json below --min-version — anywhere in the
filesystem, not just /app.

It also asserts the archive IS a docker-save archive (manifest.json
present). An OCI layout (index.json / oci-layout) is refused with a message
saying exactly that, so a format change can never silently pass this gate.

Usage:
  gate_image_archive.py --archive IMAGE.tar [--package P] [--min-version V]
"""

import argparse
import io
import json
import os
import re
import sys
import tarfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from patch_bundled import below  # noqa: E402  (shared version comparison)


def die(msg):
    print(f"gate_image_archive: FAIL: {msg}", file=sys.stderr)
    sys.exit(1)


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
    except tarfile.TarError as e:
        die(f"cannot read layer {layer_name}: {e}")
    return copies


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--archive", required=True)
    p.add_argument("--package", default="http-cache-semantics")
    p.add_argument("--min-version", default="4.3.0")
    args = p.parse_args(argv)

    try:
        tf = tarfile.open(args.archive, "r:")
    except (tarfile.TarError, OSError) as e:
        die(f"cannot open {args.archive} as a tar archive: {e}")
    with tf:
        names = set(tf.getnames())
        if "manifest.json" not in names:
            if "index.json" in names or "oci-layout" in names:
                die(f"{args.archive} is an OCI layout, not a docker-save archive "
                    "(refusing: this gate covers docker-save layer tars)")
            die(f"{args.archive} has no manifest.json — not a docker-save image tar")
        manifest = json.load(tf.extractfile("manifest.json"))
        if not isinstance(manifest, list) or not manifest or \
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
