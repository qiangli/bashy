#!/usr/bin/env python3
"""Run the TaaS (commands and folders as a service) scenario once on a claimed Linux gate lane."""
import argparse
import hashlib
import os
from pathlib import Path
import shutil
import subprocess
import sys
import uuid

HERE = Path(__file__).resolve().parent
ROOT = HERE.parents[1]
GS = ROOT / "tests/group-shaping-e2e"
PASS = "PASS taas a-o-c"


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--evidence", type=Path, required=True, help="new raw log file")
    args = parser.parse_args()
    if sys.platform != "linux" or os.environ.get("GROUP_SHAPING_GATE") != "1":
        parser.error("run on a claimed Linux gate lane with GROUP_SHAPING_GATE=1")
    lock = dict(line.split("=", 1) for line in
                (ROOT / "tools/bashy-build/image.lock").read_text().splitlines()
                if line and not line.startswith("#"))
    oci = ["bashy", "oci"]
    name = "taas-e2e-" + uuid.uuid4().hex
    args.evidence.parent.mkdir(parents=True, exist_ok=True)
    with args.evidence.open("x") as log:
        def record(message):
            print(message, file=log, flush=True)
            print(message, flush=True)

        for path in (Path(__file__), HERE / "sandbox.py", HERE / "codex", GS / "sandbox.py", ROOT / "dag.md"):
            record(f"{path.name} sha256={hashlib.sha256(path.read_bytes()).hexdigest()}")
        image = lock["base_image"]
        info = subprocess.check_output(oci + ["image", "inspect", "--format",
                                              "{{.Digest}} {{.Id}}", image], text=True, timeout=45).split()
        record(f"image {image} {' '.join(info)}")
        if len(info) != 2 or info[0] != lock["base_digest"]:
            raise RuntimeError("cached base image differs from image.lock")
        binary_dir = Path(os.environ["GROUP_SHAPING_BIN"]).resolve()
        git = shutil.which("git")
        if not git:
            raise RuntimeError("the gate lane needs an Ubuntu-compatible git executable")
        mounts = {"/fixture": HERE, "/group-shaping": GS,
                  "/python": Path(os.environ["GROUP_SHAPING_PYTHON"]),
                  "/go": Path(subprocess.check_output(["go", "env", "GOROOT"], text=True).strip()),
                  "/tools/git": Path(git)}
        for tool in ("bashy", "outpost", "delegate", "adapter-llm", "adapter-offer", "adapter-files"):
            binary = binary_dir / tool
            with binary.open("rb") as stream:
                digest = hashlib.file_digest(stream, "sha256").hexdigest()
            record(f"{tool} sha256={digest}")
            mounts["/tools/" + tool] = binary
        argv = oci + ["run", "--rm", "--name", name, "--pull=never", "--network=none",
                      "--cap-drop=all", "--security-opt=no-new-privileges", "--cpus=4",
                      "--entrypoint=/python/bin/python3", "-w", "/tmp"]
        for var in ("TAAS_T1", "TAAS_T2", "TAAS_WAIT"):
            if var in os.environ:
                argv.extend(["-e", f"{var}={os.environ[var]}"])
        for dest, source in mounts.items():
            argv.extend(["-v", f"{source.resolve(strict=True)}:{dest}:ro"])
        try:
            proc = subprocess.run(argv + [info[1], "-u", "/fixture/sandbox.py"],
                                  stdout=log, stderr=subprocess.STDOUT, timeout=int(os.environ.get("TAAS_TIMEOUT", "420")))
            if proc.returncode or PASS not in args.evidence.read_text().splitlines():
                raise RuntimeError(f"scenario failed (exit {proc.returncode}), see {args.evidence}")
        finally:
            subprocess.run(oci + ["rm", "-f", "--ignore", name], stdout=log,
                           stderr=subprocess.STDOUT, timeout=45, check=True)
        print(f"{PASS}; evidence {args.evidence}")


if __name__ == "__main__":
    main()
