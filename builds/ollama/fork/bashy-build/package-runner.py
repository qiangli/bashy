#!/usr/bin/env python3
"""Inventory and deterministic packaging; neither operation admits a release."""
import gzip
import hashlib
import json
from pathlib import Path
import subprocess
import sys


def digest(data):
    return hashlib.sha256(data).hexdigest()


def write(path, data):
    Path(path).write_text(json.dumps(data, sort_keys=True, indent=2) + "\n")


def main(args):
    mode, *args = args
    if mode == "inventory":
        src, output = args
        names = subprocess.check_output(["git", "-C", src, "ls-files", "-z"]).decode().split("\0")
        write(output, {"schema": "bashy-runner-source-inputs-v1", "files": {
            name: digest((Path(src) / name).read_bytes()) for name in names if name
        }})
    elif mode == "package":
        binary, compressed, manifest, target, commit, toolchain = args
        data = Path(binary).read_bytes()
        if not data:
            raise ValueError("empty runner")
        # Empty filename and mtime=0: no host path or clock enters the gzip header.
        with open(compressed, "wb") as out:
            with gzip.GzipFile(filename="", mode="wb", fileobj=out, compresslevel=9, mtime=0) as stream:
                stream.write(data)
        write(manifest, {
            "schema": "bashy-official-runner-v1", "target": target,
            "upstream_repo": "https://github.com/ollama/ollama",
            "upstream_tag": "v0.21.2", "upstream_commit": commit,
            "entrypoint": "./cmd/runner", "go_toolchain": toolchain,
            "cgo_enabled": True, "gowork": "off",
            "executable": Path(binary).name, "gzip": Path(compressed).name,
            "runner_sha256": digest(data), "gzip_sha256": digest(Path(compressed).read_bytes()),
            "release_admission": "PENDING_NATIVE_AND_SECURITY_GATES",
        })
    else:
        raise ValueError("unknown mode")


if __name__ == "__main__":
    main(sys.argv[1:])
