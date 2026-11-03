#!/usr/bin/env python3
"""Refresh full src fetch revisions from the exact standalone go.mod graph.

Run after dependency bumps: python3 scripts/update-src-pins.py --write.
Without --write, reject stale metadata. This downloads module metadata only;
it never builds a product or reads sibling checkout HEADs.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parent.parent
PIN = re.compile(r'(?m)^(\s*"([^"\n]+)":\s*\{"([^"\n]+)", "([^"\n]*)", ")([0-9a-f]{40})("\},)$')


def full_revision(module, version, download):
    if download.get("Path") != module or download.get("Version") != version:
        raise ValueError(f"resolved identity mismatch for {module}@{version}")
    origin = download.get("Origin", {})
    commit = origin.get("Hash", "")
    repository = re.sub(r"/v[0-9]+$", "", module)
    if origin.get("VCS") != "git" or origin.get("URL") != "https://" + repository:
        raise ValueError(f"unexpected source origin for {module}")
    if not re.fullmatch(r"[0-9a-f]{40}", commit) or not version.endswith("-" + commit[:12]):
        raise ValueError(f"full revision does not match pseudo-version for {module}")
    return commit


def refresh(source, query):
    matches = list(PIN.finditer(source))
    if {m[2] for m in matches} != {"yoke", "coreutils", "sh", "filebrowser"} or len(matches) != 4:
        raise ValueError("expected exactly four source module pin records")
    replacements = {}
    for m in matches:
        module, upstream = m[3], m[4]
        linked = query(["list", "-mod=readonly", "-m", "-json", upstream or module])
        while "Replace" in linked:
            linked = linked["Replace"]
        version = linked.get("Version", "")
        if linked.get("Path") != module or not version or version == "(devel)":
            raise ValueError(f"{m[2]} is not the expected published module")
        downloaded = query(["list", "-mod=readonly", "-m", "-json", module + "@" + version])
        replacements[m[2]] = full_revision(module, version, downloaded)
    return PIN.sub(lambda m: m[1] + replacements[m[2]] + m[6], source)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--write", action="store_true")
    args = parser.parse_args()
    env = dict(os.environ, GOWORK="off")

    def query(args):
        return json.loads(subprocess.check_output(["go", *args], cwd=ROOT, env=env, text=True))

    path = ROOT / "internal/agentos/src_mount.go"
    source = path.read_text()
    updated = refresh(source, query)
    if updated != source:
        if not args.write:
            raise SystemExit("source pins are stale; run python3 scripts/update-src-pins.py --write")
        path.write_text(updated)
    print("source pins match the standalone module graph")


if __name__ == "__main__":
    main()
