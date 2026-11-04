#!/usr/bin/env python3
"""Content identity of the two Linux certification link closures."""
import argparse
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys
import tarfile
import tempfile

ROOT = pathlib.Path(__file__).resolve().parent.parent
INPUT_FIELDS = (
    "GoFiles", "CgoFiles", "CFiles", "CXXFiles", "MFiles", "HFiles",
    "FFiles", "SFiles", "SwigFiles", "SwigCXXFiles", "SysoFiles", "EmbedFiles",
)
ENV = dict(os.environ, GOWORK="off", GOOS="linux", GOARCH="amd64", GOAMD64="v1",
           CGO_ENABLED="0", GOFLAGS="", GOEXPERIMENT="", GOFIPS140="off")
BUILDS = (("bash", "./cmd/bash", []),
          ("bashy", "./cmd/bashy", ["bashy_cert_base"]))


def output(*args, cwd=None):
    return subprocess.check_output(args, cwd=cwd, env=ENV, text=True)


def objects(stream):
    decoder = json.JSONDecoder()
    while stream.strip():
        stream = stream.lstrip()
        value, end = decoder.raw_decode(stream)
        yield value
        stream = stream[end:]


def closure(root):
    version = output("go", "env", "GOVERSION").strip()
    driver = (root / "tools/productgo/main.go").read_text()
    original = pathlib.Path(output("go", "env", "GOROOT").strip(),
                            "src/runtime/signal_unix.go").read_bytes()
    expected = re.search(r'const sourceHash = "([0-9a-f]{64})"', driver).group(1)
    if hashlib.sha256(original).hexdigest() != expected:
        raise RuntimeError("unrecognized Go runtime source for product overlay")
    insertion = re.search(r'const insertion = `([^`]*)`', driver).group(1)
    helpers = re.search(r'const helpers = `([^`]*)`', driver).group(1)
    anchor = "\tt := &sigtable[sig]\n\tif t.flags&_SigSetStack != 0 {"
    source = original.decode()
    if source.count(anchor) != 1:
        raise RuntimeError("product runtime patch anchor not unique")
    patched = source.replace(anchor, "\tt := &sigtable[sig]\n" + insertion +
                             "\tif t.flags&_SigSetStack != 0 {", 1) + helpers
    overlay_hash = hashlib.sha256(patched.encode()).hexdigest()
    build_tool_inputs = {name: hashlib.sha256((root / name).read_bytes()).hexdigest()
                         for name in ("scripts/go-product.sh", "tools/productgo/main.go")}
    result = {"schema": "bashy.cert-closure.v1", "environment": {
        "GOOS": "linux", "GOARCH": "amd64", "GOAMD64": "v1", "CGO_ENABLED": "0",
        "GOWORK": "off", "GOFLAGS": "", "GOEXPERIMENT": "", "GOFIPS140": "off"},
        "toolchain": version, "builds": {}}
    for name, target, tags in BUILDS:
        actual_tags = tags + ["bashy_runtime_snapshot"]
        flags = {"target": target, "tags": actual_tags, "gcflags": [], "asmflags": [],
                 "build_flags": ["-trimpath", "-overlay=product-runtime"],
                 "ldflags": (["-s", "-w"] if name == "bash" else ["-w"]) +
                            ["-X runtime.bashyInheritedIgnore=1"]}
        command = ["go", "list", "-deps", "-json"]
        command += ["-tags=" + ",".join(actual_tags)]
        command.append(target)
        packages = {}
        for package in objects(output(*command, cwd=root)):
            import_path = package["ImportPath"]
            directory = pathlib.Path(package["Dir"])
            files = {}
            for field in INPUT_FIELDS:
                for filename in package.get(field, []):
                    path = directory / filename
                    if not path.is_file():
                        raise RuntimeError("missing compiled input: " + str(path))
                    files[filename] = (overlay_hash if import_path == "runtime" and
                                       filename == "signal_unix.go" else
                                       hashlib.sha256(path.read_bytes()).hexdigest())
            packages[import_path] = {"files": dict(sorted(files.items()))}
        identity = {"toolchain": version, "environment": result["environment"],
                    "flags": flags, "build_tool_inputs": build_tool_inputs,
                    "packages": packages}
        digest = hashlib.sha256(json.dumps(identity, sort_keys=True,
                                            separators=(",", ":")).encode()).hexdigest()
        result["builds"][name] = {"digest": digest, "flags": flags,
                                   "build_tool_inputs": build_tool_inputs,
                                   "packages": packages}
    return result


def at_revision(revision):
    resolved = output("git", "rev-parse", "--verify", revision + "^{commit}", cwd=ROOT).strip()
    temp = tempfile.TemporaryDirectory(prefix="cert-closure-")
    proc = subprocess.Popen(["git", "archive", "--format=tar", resolved], cwd=ROOT,
                            stdout=subprocess.PIPE)
    try:
        with tarfile.open(fileobj=proc.stdout, mode="r|") as archive:
            archive.extractall(temp.name)
        if proc.wait() != 0:
            raise RuntimeError("git archive failed for " + resolved)
        return closure(pathlib.Path(temp.name))
    finally:
        proc.stdout.close()
        if proc.poll() is None:
            proc.kill()
            proc.wait()
        temp.cleanup()


def changes(before, after):
    for name in ("bash", "bashy"):
        old = before["builds"][name]
        new = after["builds"][name]
        if old["digest"] == new["digest"]:
            print(name + ": SAME " + new["digest"])
            continue
        print(name + ": CHANGED " + old["digest"] + " -> " + new["digest"])
        if before.get("toolchain") != after.get("toolchain"):
            print("  toolchain: " + str(before.get("toolchain")) + " -> " + str(after.get("toolchain")))
        if before.get("environment") != after.get("environment"):
            print("  build environment changed")
        if old.get("flags") != new.get("flags"):
            print("  build flags changed")
        for path in sorted(old.get("build_tool_inputs", {}).keys() |
                           new.get("build_tool_inputs", {}).keys()):
            if old.get("build_tool_inputs", {}).get(path) != new.get("build_tool_inputs", {}).get(path):
                print("  build tool " + path + " changed")
        prior = old["packages"]
        current = new["packages"]
        for package in sorted(prior.keys() | current.keys()):
            if package not in prior or package not in current:
                print("  package " + package + (" added" if package in current else " removed"))
                continue
            old_files = prior[package]["files"]
            new_files = current[package]["files"]
            for filename in sorted(old_files.keys() | new_files.keys()):
                if old_files.get(filename) != new_files.get(filename):
                    print("  " + package + "/" + filename + ": " +
                          ("added" if filename not in old_files else
                           "removed" if filename not in new_files else "modified"))


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--rev", help="calculate from an archived Git revision")
    parser.add_argument("--check", help="compare with a manifest JSON file")
    args = parser.parse_args()
    result = at_revision(args.rev) if args.rev else closure(ROOT)
    if not args.check:
        print(json.dumps(result, sort_keys=True))
        return 0
    before = json.loads(pathlib.Path(args.check).read_text())["cert_closure"]
    changes(before, result)
    return int(any(before["builds"][name]["digest"] != result["builds"][name]["digest"]
                   for name, _, _ in BUILDS))


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, KeyError, RuntimeError, subprocess.CalledProcessError) as error:
        print("cert closure: " + str(error), file=sys.stderr)
        sys.exit(2)
