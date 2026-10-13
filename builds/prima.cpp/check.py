#!/usr/bin/env python3
"""Inspect a blocked source candidate. Never configure, compile or execute it."""
import argparse
import hashlib
import json
from pathlib import Path
import re
import subprocess
import sys


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(candidate):
    require(candidate["schema"] == "bashy-blocked-source-candidate-v1", "unknown schema")
    require(candidate["tool"] == "prima.cpp", "wrong tool")
    require(candidate["status"] == "blocked", "this inspector cannot authorize a release")
    require(candidate["source"]["official_release_tag"] is None, "release admission needs a reviewed implementation")
    require(len(candidate["blockers"]) > 0, "missing blockers")
    for source in [candidate["source"], *candidate["dependency_candidates"]]:
        require(re.fullmatch(r"[0-9a-f]{40}", source["commit"]), "full commit required")
        require(source["license"] == "MIT", "unexpected candidate license")
    require(candidate["source"]["commit"] != candidate["rejected_release"]["commit"], "unsafe old release selected")


def git(root, *args):
    return subprocess.check_output(["git", "-C", str(root), *args], text=True).strip()


def inspect(root, source):
    require(git(root, "rev-parse", "HEAD") == source["commit"], "checkout differs from exact candidate pin")
    require(not git(root, "status", "--porcelain", "--untracked-files=all", "--ignored"), "source checkout is not pristine")
    license_path = Path(root) / source["license_path"]
    require(not license_path.is_symlink(), "license must be a regular source file")
    digest = hashlib.sha256(license_path.read_bytes()).hexdigest()
    if "license_sha256" in source:
        require(digest == source["license_sha256"], "license digest mismatch")
    if "license_git_blob" in source:
        require(git(root, "hash-object", source["license_path"]) == source["license_git_blob"], "license blob mismatch")
    if "tag" in source:
        require(git(root, "rev-parse", "refs/tags/" + source["tag"] + "^{commit}") == source["commit"], "dependency tag differs from pin")
    return {"repository": source["repository"], "commit": source["commit"], "license_sha256": digest}


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--manifest", type=Path, default=Path(__file__).with_name("candidate.json"))
    parser.add_argument("--source", type=Path)
    parser.add_argument("--nng", type=Path)
    parser.add_argument("--require-release", action="store_true")
    args = parser.parse_args()
    try:
        candidate = json.loads(args.manifest.read_text())
        validate(candidate)
        inspected = []
        if args.source:
            inspected.append(inspect(args.source, candidate["source"]))
        if args.nng:
            inspected.append(inspect(args.nng, candidate["dependency_candidates"][0]))
        print(json.dumps({"status": "blocked", "source_inspection": inspected,
                          "build_permitted": False, "blockers": candidate["blockers"]}, indent=2))
        if args.require_release:
            print("prima.cpp release refused: no admissible stable upstream pin; see blockers", file=sys.stderr)
            return 1
        return 0
    except (ValueError, KeyError, OSError, subprocess.CalledProcessError) as error:
        print(f"prima.cpp inspection failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    sys.exit(main())
