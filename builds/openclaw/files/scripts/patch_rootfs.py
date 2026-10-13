#!/usr/bin/env python3
"""patch_rootfs.py — full-rootfs security patch for GHSA-ch52-4w7c-c8xp.

Operates on an EXTRACTED container rootfs directory (the flattened merged
view from `docker export` of the layered shipped image), not on a tarball:
finds EVERY node_modules/<pkg>/package.json anywhere under the root —
including copies OUTSIDE /app such as the Node base's own
/usr/local/lib/node_modules/npm/node_modules/<pkg> — and replaces each copy
below --min-version with the pinned fixed release.

Fail-closed both ways: patch mode REFUSES a silent no-op (callers branch on
--verify-only first, exactly like patch_bundled.py); a mismatching
--integrity aborts before the rootfs is touched.

Usage (gate):
  patch_rootfs.py --verify-only --rootfs DIR
Usage (patch):
  patch_rootfs.py --rootfs DIR --replacement PKG.tgz --integrity sha512-BASE64
"""

import argparse
import base64
import hashlib
import io
import json
import os
import re
import shutil
import sys
import tarfile

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from patch_bundled import below  # noqa: E402  (shared version comparison)


def die(msg):
    print(f"patch_rootfs: FAIL: {msg}", file=sys.stderr)
    sys.exit(1)


def pkg_json_re(pkg):
    return re.compile(r"(?:^|/)node_modules/" + re.escape(pkg) + r"/package\.json$")


def find_copies(root, pkg):
    """[(rel_dir, version)] for every node_modules/<pkg>/package.json under root."""
    rx = pkg_json_re(pkg)
    found = []
    for dirpath, _dirnames, filenames in os.walk(root):
        if "package.json" not in filenames:
            continue
        full = os.path.join(dirpath, "package.json")
        rel = os.path.relpath(full, root)
        if not rx.search(rel):
            continue
        try:
            with open(full, "rb") as f:
                version = json.load(f).get("version")
        except (ValueError, UnicodeDecodeError, OSError):
            version = None
        found.append((os.path.dirname(rel), version))
    return sorted(found)


def load_replacement(path, integrity, minimum, package):
    raw = open(path, "rb").read()
    m = re.match(r"^sha512-([A-Za-z0-9+/]+={0,2})$", (integrity or "").strip())
    if not m:
        die("--integrity must be a sha512-BASE64 value")
    if base64.b64encode(hashlib.sha512(raw).digest()).decode() != m.group(1):
        die("replacement tarball does not match the pinned sha512 integrity; refusing")
    files = []
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as tf:
        for rm in tf.getmembers():
            if not rm.name.startswith("package/") or not (rm.isfile() or rm.isdir() or rm.issym()):
                continue
            rel = rm.name[len("package/"):]
            if rel:
                files.append((rel, rm, tf.extractfile(rm).read() if rm.isfile() else None))
    version = None
    for rel, _, data in files:
        if rel == "package.json":
            version = json.loads(data.decode("utf-8")).get("version")
    if version is None:
        die("replacement tarball has no package/package.json version")
    if below(version, minimum):
        die(f"replacement is {package}@{version}, below {minimum}")
    return files, version


def verify_only(args):
    copies = find_copies(args.rootfs, args.package)
    bad = [(d, v) for d, v in copies if below(v, args.min_version)]
    for d, v in copies:
        print(f"{d}: {args.package}@{v}")
    if bad:
        die(f"{len(bad)} copy/copies of {args.package} below {args.min_version} remain: "
            + ", ".join(f"{d}@{v}" for d, v in bad))
    print(f"patch_rootfs: OK — no {args.package} < {args.min_version} in {args.rootfs} "
          f"({len(copies)} copy/copies checked)")


def clear_dir(path):
    for entry in os.listdir(path):
        full = os.path.join(path, entry)
        if os.path.islink(full) or os.path.isfile(full):
            os.remove(full)
        elif os.path.isdir(full):
            shutil.rmtree(full)
        else:
            die(f"refusing to remove special file {full}")


def write_replacement(root, target, files):
    dest = os.path.join(root, target)
    clear_dir(dest)
    for rel, rm, rdata in files:
        out = os.path.join(dest, rel)
        if rm.isdir():
            os.makedirs(out, exist_ok=True)
            os.chmod(out, rm.mode)
        elif rm.issym():
            if os.path.lexists(out):
                os.remove(out)
            os.symlink(rm.linkname, out)
        elif rm.isfile():
            os.makedirs(os.path.dirname(out), exist_ok=True)
            with open(out, "wb") as f:
                f.write(rdata)
            os.chmod(out, rm.mode)
        try:
            os.chown(out, rm.uid, rm.gid)
        except (PermissionError, OSError):
            pass  # best effort; CI runs as root so this normally sticks


def patch(args):
    copies = find_copies(args.rootfs, args.package)
    targets = [d for d, v in copies if below(v, args.min_version)]
    if not targets:
        die(f"no {args.package} below {args.min_version} found; nothing to patch "
            "(refusing a silent no-op — check the rootfs)")
    files, new_version = load_replacement(args.replacement, args.integrity,
                                          args.min_version, args.package)
    for t in targets:
        if os.path.islink(os.path.join(args.rootfs, t)) or \
                not os.path.isdir(os.path.join(args.rootfs, t)):
            die(f"refusing to patch non-directory {t}")
    for t in targets:
        write_replacement(args.rootfs, t, files)
        print(f"patched {t}: {args.package} -> {new_version}")
    left = [(d, v) for d, v in find_copies(args.rootfs, args.package)
            if below(v, args.min_version)]
    if left:
        die(f"rootfs still holds {args.package} below {args.min_version}: {left}")
    print(f"patch_rootfs: OK — {len(targets)} copy/copies replaced")


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__,
                                formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--rootfs", required=True)
    p.add_argument("--replacement")
    p.add_argument("--integrity")
    p.add_argument("--package", default="http-cache-semantics")
    p.add_argument("--min-version", default="4.3.0")
    p.add_argument("--verify-only", action="store_true")
    args = p.parse_args(argv)
    if not os.path.isdir(args.rootfs):
        die(f"--rootfs {args.rootfs} is not a directory")
    if args.verify_only:
        verify_only(args)
    else:
        if not args.replacement or not args.integrity:
            die("patch mode needs --replacement and --integrity")
        patch(args)


if __name__ == "__main__":
    main()
