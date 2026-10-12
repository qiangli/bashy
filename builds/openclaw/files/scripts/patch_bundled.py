#!/usr/bin/env python3
"""patch_bundled.py — post-pack security patch for GHSA-ch52-4w7c-c8xp.

After upstream's scripts/package-openclaw-for-docker.mjs writes the gateway
tarball and BEFORE any signing, replace every bundled copy of a package below
the fixed version (npm 12.x bundles its own http-cache-semantics 4.2.0, and
bundled dependencies ignore overrides) with the registry tarball of the fixed
release, then update the CycloneDX SBOM.

Python's tarfile reads and writes GNU LongLink and PAX entries natively (the
upstream tarball uses GNU long-path entries). Fail-closed throughout: a
mismatching integrity, an unreadable version, or any unexpected change to an
entry outside the patched directories aborts without touching the tarball.

Usage (patch):
  patch_bundled.py --tarball IN_OUT.tgz --replacement PKG.tgz \
    --integrity sha512-BASE64 [--sbom CYCLONEDX.json] \
    [--package http-cache-semantics] [--min-version 4.3.0]
Usage (gate, after the patch and before signing):
  patch_bundled.py --verify-only --tarball IN_OUT.tgz \
    [--package http-cache-semantics] [--min-version 4.3.0]
"""

import argparse
import base64
import gzip
import hashlib
import io
import json
import os
import re
import sys
import tarfile
import tempfile

NOTE = "bashy security patch GHSA-ch52-4w7c-c8xp"


def die(msg):
    print(f"patch_bundled: FAIL: {msg}", file=sys.stderr)
    sys.exit(1)


def vtuple(v):
    m = re.match(r"^\s*v?(\d+)\.(\d+)\.(\d+)", str(v or ""))
    return tuple(int(x) for x in m.groups()) if m else None


def below(v, minimum):
    t = vtuple(v)
    return t is None or t < vtuple(minimum)


def read_tar(path):
    """Return the ordered members with their file bytes (None for non-files)."""
    out = []
    with tarfile.open(path, "r:gz") as tf:
        for m in tf.getmembers():
            data = tf.extractfile(m).read() if m.isfile() else None
            out.append((m, data))
    return out


def pkg_json_re(pkg):
    return re.compile(r"^(?P<dir>(?:.*/)?node_modules/" + re.escape(pkg) + r")/package\.json$")


def bundled_copies(members, pkg):
    """[(dir, version)] for every node_modules/<pkg>/package.json in the tarball."""
    rx = pkg_json_re(pkg)
    found = []
    for m, data in members:
        mm = rx.match(m.name)
        if not mm or data is None:
            continue
        try:
            version = json.loads(data.decode("utf-8")).get("version")
        except (ValueError, UnicodeDecodeError):
            version = None
        found.append((mm.group("dir"), version))
    return found


def under(name, prefix):
    return name == prefix or name.startswith(prefix + "/")


def digest(m, data):
    return (m.name, m.type, m.mode, m.size, m.linkname,
            hashlib.sha256(data).hexdigest() if data is not None else None)


def verify_only(args):
    members = read_tar(args.tarball)
    copies = bundled_copies(members, args.package)
    bad = [(d, v) for d, v in copies if below(v, args.min_version)]
    for d, v in copies:
        print(f"{d}: {args.package}@{v}")
    if bad:
        die(f"{len(bad)} copy/copies of {args.package} below {args.min_version} remain: "
            + ", ".join(f"{d}@{v}" for d, v in bad))
    print(f"patch_bundled: OK — no {args.package} < {args.min_version} in {args.tarball} "
          f"({len(copies)} copy/copies checked)")


def load_replacement(path, integrity):
    raw = open(path, "rb").read()
    m = re.match(r"^sha512-([A-Za-z0-9+/]+={0,2})$", (integrity or "").strip())
    if not m:
        die("--integrity must be a sha512-BASE64 value")
    if base64.b64encode(hashlib.sha512(raw).digest()).decode() != m.group(1):
        die("replacement tarball does not match the pinned sha512 integrity; refusing")
    files = []
    with tarfile.open(fileobj=io.BytesIO(raw), mode="r:gz") as tf:
        for rm in tf.getmembers():
            if not rm.name.startswith("package/") or not (rm.isfile() or rm.isdir()):
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
    return files, version


def patch(args):
    members = read_tar(args.tarball)
    copies = bundled_copies(members, args.package)
    targets = [d for d, v in copies if below(v, args.min_version)]
    if not targets:
        die(f"no {args.package} below {args.min_version} found; nothing to patch "
            "(refusing a silent no-op — check the tarball)")
    files, new_version = load_replacement(args.replacement, args.integrity)
    if below(new_version, args.min_version):
        die(f"replacement is {args.package}@{new_version}, below {args.min_version}")

    out_members = []
    inserted = set()
    for m, data in members:
        target = next((t for t in targets if under(m.name, t)), None)
        if target is None:
            out_members.append((m, data))
            continue
        if target in inserted:
            continue
        inserted.add(target)
        # Insert the fixed package where the old one stood, inheriting the
        # original directory entry's owner and timestamp.
        tmpl = m
        if m.isdir() and m.name.rstrip("/") == target:
            out_members.append((m, None))  # keep the package directory entry itself
        for rel, rm, rdata in files:
            ti = tarfile.TarInfo(f"{target}/{rel}")
            ti.type = rm.type
            ti.mode = rm.mode
            ti.mtime = tmpl.mtime
            ti.uid, ti.gid, ti.uname, ti.gname = tmpl.uid, tmpl.gid, tmpl.uname, tmpl.gname
            ti.size = len(rdata) if rdata is not None else 0
            out_members.append((ti, rdata))

    fd, tmp_path = tempfile.mkstemp(dir=os.path.dirname(os.path.abspath(args.tarball)), suffix=".tgz")
    os.close(fd)
    try:
        with open(tmp_path, "wb") as raw_out:
            with gzip.GzipFile(filename="", mode="wb", fileobj=raw_out, mtime=0) as gz:
                with tarfile.open(fileobj=gz, mode="w", format=tarfile.GNU_FORMAT) as tf:
                    for m, data in out_members:
                        tf.addfile(m, io.BytesIO(data) if data is not None else None)

        # Assert: every entry outside the patched directories is unchanged, in order.
        before = [digest(m, d) for m, d in members if not any(under(m.name, t) for t in targets)]
        after_members = read_tar(tmp_path)
        after = [digest(m, d) for m, d in after_members if not any(under(m.name, t) for t in targets)]
        if before != after:
            die("an entry outside the patched directories changed; refusing to write")
        left = [(d, v) for d, v in bundled_copies(after_members, args.package) if below(v, args.min_version)]
        if left:
            die(f"patched tarball still holds {args.package} below {args.min_version}: {left}")
        os.replace(tmp_path, args.tarball)
    finally:
        if os.path.exists(tmp_path):
            os.remove(tmp_path)

    for t in targets:
        print(f"patched {t}: {args.package} -> {new_version}")
    if args.sbom:
        update_sbom(args.sbom, args.package, new_version, args.min_version)
    print(f"patch_bundled: OK — {len(targets)} copy/copies replaced; {len(before)} other entries unchanged")


def update_sbom(path, pkg, new_version, minimum):
    doc = json.load(open(path))
    comps = doc.get("components")
    if not isinstance(comps, list):
        die(f"SBOM {path} has no components[]; refusing to guess its shape")
    renames = {}
    changed = 0

    def walk(items):
        nonlocal changed
        for c in items:
            if c.get("name") == pkg and below(c.get("version"), minimum):
                old = str(c.get("version"))
                c["version"] = new_version
                purl = c.get("purl")
                if isinstance(purl, str) and purl.startswith(f"pkg:npm/{pkg}@"):
                    q = purl[purl.index("?"):] if "?" in purl else ""
                    c["purl"] = f"pkg:npm/{pkg}@{new_version}{q}"
                ref = c.get("bom-ref")
                if isinstance(ref, str) and old in ref:
                    c["bom-ref"] = ref.replace(old, new_version)
                    renames[ref] = c["bom-ref"]
                props = c.setdefault("properties", [])
                props.append({"name": "bashy:note", "value": NOTE})
                changed += 1
            if isinstance(c.get("components"), list):
                walk(c["components"])

    walk(comps)
    for dep in doc.get("dependencies", []) or []:
        if dep.get("ref") in renames:
            dep["ref"] = renames[dep["ref"]]
        if isinstance(dep.get("dependsOn"), list):
            dep["dependsOn"] = [renames.get(r, r) for r in dep["dependsOn"]]
    with open(path, "w") as f:
        json.dump(doc, f, indent=2)
        f.write("\n")
    print(f"SBOM: {changed} {pkg} component(s) updated to {new_version}")


def main(argv=None):
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    p.add_argument("--tarball", required=True)
    p.add_argument("--replacement")
    p.add_argument("--integrity")
    p.add_argument("--sbom")
    p.add_argument("--package", default="http-cache-semantics")
    p.add_argument("--min-version", default="4.3.0")
    p.add_argument("--verify-only", action="store_true")
    args = p.parse_args(argv)
    if args.verify_only:
        verify_only(args)
    else:
        if not args.replacement or not args.integrity:
            die("patch mode needs --replacement and --integrity")
        patch(args)


if __name__ == "__main__":
    main()
