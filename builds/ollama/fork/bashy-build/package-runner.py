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


def backend_record(identity_path, directory, commit):
    """Bind each backend shared library (digest + size) to a verified source/build identity."""
    identity = json.loads(identity_path.read_text())
    if identity.get("source_commit") != commit:
        raise ValueError("backend source commit does not match the pinned upstream commit")
    for key in ("cuda_toolkit", "nvcc_sha256", "preset", "toolkit_sha256", "toolkit_version", "toolkit_components"):
        if not identity.get(key):
            raise ValueError("backend identity missing " + key)
    artifacts = []
    for name in identity.get("artifacts", []):
        # The toolkit is a build-only SDK; driver/cudart/cuBLAS are host-provided
        # at runtime and must never ship as backend artifacts.
        if any(b in Path(name).name for b in ("libcudart", "libcublas", "libcuda.so")):
            raise ValueError("host-provided CUDA runtime must not be bundled: " + name)
        path = Path(directory) / name
        if path.is_symlink() or not path.is_file() or path.stat().st_size == 0:
            raise ValueError("backend artifact must be a real nonempty file: " + name)
        artifacts.append({"name": name, "kind": "cuda-shared-library",
                          "sha256": digest(path.read_bytes()), "size": path.stat().st_size})
    if not artifacts:
        raise ValueError("CUDA target requires at least one backend shared library")
    return {"backend_artifacts": artifacts,
            "backend_identity": {k: identity[k] for k in ("source_commit", "cuda_toolkit", "nvcc_sha256", "preset", "toolkit_sha256", "toolkit_version", "toolkit_components")},
            "toolkit_role": "build-only; runtime host-provided",
            "release_admission": "PENDING_CUDA_TOOLCHAIN_LICENSE_AND_AUDIT_REVIEW"}


def main(args):
    mode, *args = args
    if mode == "inventory":
        src, output = args
        names = subprocess.check_output(["git", "-C", src, "ls-files", "-z"]).decode().split("\0")
        write(output, {"schema": "bashy-runner-source-inputs-v1", "files": {
            name: digest((Path(src) / name).read_bytes()) for name in names if name
        }})
    elif mode == "package":
        binary, compressed, manifest, target, commit, toolchain, *backend = args
        if len(backend) > 1:
            raise ValueError("at most one backend identity file")
        data = Path(binary).read_bytes()
        if not data:
            raise ValueError("empty runner")
        # Empty filename and mtime=0: no host path or clock enters the gzip header.
        with open(compressed, "wb") as out:
            with gzip.GzipFile(filename="", mode="wb", fileobj=out, compresslevel=9, mtime=0) as stream:
                stream.write(data)
        record = {
            "schema": "bashy-official-runner-v1", "target": target,
            "upstream_repo": "https://github.com/ollama/ollama",
            "upstream_tag": "v0.21.2", "upstream_commit": commit,
            "entrypoint": "./cmd/runner", "go_toolchain": toolchain,
            "cgo_enabled": True, "gowork": "off",
            "executable": Path(binary).name, "gzip": Path(compressed).name,
            "runner_sha256": digest(data), "gzip_sha256": digest(Path(compressed).read_bytes()),
            "release_admission": "PENDING_NATIVE_AND_SECURITY_GATES",
        }
        if backend:
            record.update(backend_record(Path(backend[0]), Path(manifest).parent, commit))
        write(manifest, record)
    else:
        raise ValueError("unknown mode")


if __name__ == "__main__":
    main(sys.argv[1:])
