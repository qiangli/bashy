#!/usr/bin/env python3
"""release-assets-check.py DIST "STATIC ASSETS": manifest-derived release set.

Every backend artifact named by any dist manifest (backend_artifacts) must be
present as a real file whose sha256 matches, so it is published and signed with
the rest. A shared library in dist that no manifest lists fails closed.
Prints the full derived asset list.
"""
import hashlib
import json
from pathlib import Path
import sys


def main(dist, static):
    dist = Path(dist)
    required = set(static.split())
    needs_admission = False
    for manifest in sorted(dist.glob("*.json")):
        try:
            data = json.loads(manifest.read_text())
        except ValueError:
            continue
        if not isinstance(data, dict):
            continue
        for item in data.get("backend_artifacts") or []:
            needs_admission = True
            name = item.get("name", "")
            path = dist / name
            if not name or "/" in name or path.is_symlink() or not path.is_file():
                sys.exit(f"release-assets-check: backend artifact missing or not a real file: {name!r}")
            if hashlib.sha256(path.read_bytes()).hexdigest() != item.get("sha256"):
                sys.exit(f"release-assets-check: backend artifact digest differs from manifest: {name}")
            if name not in required:
                sys.exit(f"release-assets-check: backend artifact packaged by the build but missing from the release asset list: {name}")
    if needs_admission:
        # A CUDA backend ships only with the reviewed admission result for THIS run.
        record = dist / "runner-cuda-admission.json"
        if "runner-cuda-admission.json" not in required or not record.is_file():
            sys.exit("release-assets-check: CUDA backend packaged but runner-cuda-admission.json is missing from the release")
        try:
            admitted = json.loads(record.read_text()).get("admission")
        except ValueError:
            admitted = None
        if admitted != "PRERELEASE_ADMITTED":
            sys.exit(f"release-assets-check: CUDA backend not admitted (runner-cuda-admission.json: {admitted!r})")
    for path in sorted(dist.iterdir()):
        if path.is_file() and ".so" in path.name and path.name not in required:
            sys.exit(f"release-assets-check: shared library not listed by any manifest: {path.name}")
    print("\n".join(sorted(required)))


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit("usage: release-assets-check.py DIST 'ASSETS'")
    main(*sys.argv[1:])
