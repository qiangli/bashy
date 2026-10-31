#!/usr/bin/env python3
"""CUDA backend linkage audit and host-runtime gate (S451.4a).

link: backend must NEED libcudart and libcublas dynamically, define no
cuda*/cublas* symbols (static link), and NEED no libcuda (driver is host-only).
host: host must resolve libcuda, libcudart and libcublas; otherwise fail closed
unless --cpu-explicit asks for the CPU fallback.
"""
import argparse
import os
import re
import shutil
import subprocess
import sys

REQUIRED = ("libcudart", "libcublas")
HOST = ("libcuda.so", "libcudart.so", "libcublas.so")


def audit_link(text):
    needed = re.findall(r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]", text)
    errors = []
    for lib in REQUIRED:
        if not any(n.startswith(lib + ".so") for n in needed):
            errors.append(f"{lib} is not a dynamic NEEDED entry (static or missing)")
    for n in needed:
        if n.startswith("libcuda.so"):
            errors.append("libcuda is linked directly; the driver must be dlopen'd from the host")
    for kind, value in re.findall(r"\((RUNPATH|RPATH)\)\s+Library (?:runpath|rpath): \[([^\]]*)\]", text):
        if value != "$ORIGIN":
            errors.append(f"{kind} must be absent or exactly $ORIGIN (got [{value}]); no build or absolute paths")
    for line in text.splitlines():
        m = re.match(r"\s*\d+:\s+[0-9a-f]+\s+\d+\s+\S+\s+\S+\s+\S+\s+(\S+)\s+((?:cuda|cublas)\w*)", line)
        if m and m.group(1) != "UND":
            errors.append(f"defined NVIDIA symbol {m.group(2)}: statically linked runtime")
    return needed, errors


SYSTEM = ("libc.so", "libm.so", "libdl.so", "libpthread.so", "librt.so", "libstdc++.so", "libgcc_s.so", "ld-linux", "libcublasLt.so",
          "libcudart.so", "libcublas.so", "libnvidia-")


def transitive(needed):
    """NEEDED entries that are neither system nor host-provided NVIDIA: they must ship (e.g. libggml-base)."""
    return [n for n in needed if not n.startswith(SYSTEM)]


HOST_REQUIRED = ("libcuda", "libcudart", "libcublas", "libcublasLt", "libnv")
SHIP_PREFIX = "libggml-"

NEEDED_RE = r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]"


def closure(root_so, search, readelf):
    """Recursive non-system dependency closure of a backend library, resolved (symlinks followed, real file must stay under `search`)."""
    import os
    seen, queue, errors, host = [], [root_so], [], []
    while queue:
        so = queue.pop(0)
        text = subprocess.check_output([readelf, "-d", "--wide", so], text=True)
        for n in re.findall(NEEDED_RE, text):
            if n.startswith(("libcuda.so", "libnvidia-")):
                errors.append(f"{os.path.basename(so)} links host driver library {n}")
            elif n.startswith(HOST_REQUIRED):
                if n not in host:
                    host.append(n)
            elif n.startswith(SYSTEM) or n in seen:
                continue
            elif not n.startswith(SHIP_PREFIX):
                errors.append(f"refusing to ship non-libggml build-tree dependency {n} (needed by {os.path.basename(so)})")
            else:
                root = os.path.realpath(search)
                hits = sorted(os.path.join(r, n) for r, ds, fs in os.walk(search) for n2 in (fs + ds) if n2 == n)
                hits = [os.path.realpath(h) for h in hits]
                hits = [h for h in hits if os.path.isfile(h) and os.path.commonpath([root, h]) == root]
                if not hits:
                    errors.append(f"unresolved backend dependency {n} (needed by {os.path.basename(so)})")
                    continue
                seen.append(n)
                queue.append(hits[0])
    return seen, host, errors


def main():
    ap = argparse.ArgumentParser()
    sub = ap.add_subparsers(dest="cmd", required=True)
    link = sub.add_parser("link")
    link.add_argument("so", nargs="?")
    link.add_argument("--readelf-file")
    need = sub.add_parser("needed")
    need.add_argument("so", nargs="?")
    need.add_argument("--readelf-file")
    clo = sub.add_parser("closure")
    clo.add_argument("so")
    clo.add_argument("--search", required=True)
    clo.add_argument("--readelf", default="readelf")
    clo.add_argument("--host-required-out")
    host = sub.add_parser("host")
    host.add_argument("--ldconfig-file")
    host.add_argument("--cpu-explicit", action="store_true")
    a = ap.parse_args()
    if a.cmd == "closure":
        seen, host, errors = closure(a.so, a.search, a.readelf)
        if errors:
            sys.exit("cuda-link-audit: " + "; ".join(errors))
        if a.host_required_out:
            open(a.host_required_out, "w").write("".join(h + "\n" for h in host))
        print("\n".join(seen))
        return
    if a.cmd == "needed":
        text = open(a.readelf_file).read() if a.readelf_file else subprocess.check_output(["readelf", "-d", "--wide", a.so], text=True)
        print("\n".join(transitive(re.findall(r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]", text))))
        return
    if a.cmd == "link":
        if a.readelf_file:
            text = open(a.readelf_file).read()
        else:
            text = "".join(subprocess.check_output(["readelf", "-d", "--dyn-syms", "--wide", a.so], text=True))
        needed, errors = audit_link(text)
        if errors:
            sys.exit("cuda-link-audit: " + "; ".join(errors))
        print("cuda-link-audit: dynamic NEEDED " + " ".join(needed))
        return
    if a.ldconfig_file:
        text = open(a.ldconfig_file).read()
    else:
        found = shutil.which("ldconfig") or next((c for c in ("/usr/sbin/ldconfig", "/sbin/ldconfig") if os.path.isfile(c)), None)
        if found is None:
            if not a.cpu_explicit:
                sys.exit("cuda-link-audit: ldconfig not found (PATH, /usr/sbin, /sbin): no host CUDA runtime evidence; fails closed (pass --cpu-explicit for CPU)")
            text = ""
        else:
            text = subprocess.check_output([found, "-p"], text=True)
    missing = [h for h in HOST if h not in text]
    if not missing:
        print("cuda: host runtime present")
    elif a.cpu_explicit:
        print("cpu-explicit: host lacks " + " ".join(missing))
    else:
        sys.exit("cuda-link-audit: host runtime missing " + " ".join(missing) + "; fails closed (pass --cpu-explicit for CPU)")


main()
