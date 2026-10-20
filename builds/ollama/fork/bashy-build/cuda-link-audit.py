#!/usr/bin/env python3
"""CUDA backend linkage audit and host-runtime gate (S451.4a).

link: backend must NEED libcudart and libcublas dynamically, define no
cuda*/cublas* symbols (static link), and NEED no libcuda (driver is host-only).
host: host must resolve libcuda, libcudart and libcublas; otherwise fail closed
unless --cpu-explicit asks for the CPU fallback.
"""
import argparse
import re
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


NEEDED_RE = r"\(NEEDED\)\s+Shared library: \[([^\]]+)\]"


def closure(root_so, search, readelf):
    """Recursive non-system dependency closure of a backend library, resolved as real files under `search`."""
    import os
    seen, queue, errors = [], [root_so], []
    while queue:
        so = queue.pop(0)
        text = subprocess.check_output([readelf, "-d", "--wide", so], text=True)
        for n in re.findall(NEEDED_RE, text):
            if n.startswith(("libcuda.so", "libnvidia-")):
                errors.append(f"{os.path.basename(so)} links host driver library {n}")
            elif n.startswith(SYSTEM) or n in seen:
                continue
            else:
                hits = sorted(os.path.join(r, n) for r, _, fs in os.walk(search) if n in fs)
                hits = [h for h in hits if not os.path.islink(h)]
                if not hits:
                    errors.append(f"unresolved backend dependency {n} (needed by {os.path.basename(so)})")
                    continue
                seen.append(n)
                queue.append(hits[0])
    return seen, errors


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
    host = sub.add_parser("host")
    host.add_argument("--ldconfig-file")
    host.add_argument("--cpu-explicit", action="store_true")
    a = ap.parse_args()
    if a.cmd == "closure":
        seen, errors = closure(a.so, a.search, a.readelf)
        if errors:
            sys.exit("cuda-link-audit: " + "; ".join(errors))
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
    text = open(a.ldconfig_file).read() if a.ldconfig_file else subprocess.check_output(["ldconfig", "-p"], text=True)
    missing = [h for h in HOST if h not in text]
    if not missing:
        print("cuda: host runtime present")
    elif a.cpu_explicit:
        print("cpu-explicit: host lacks " + " ".join(missing))
    else:
        sys.exit("cuda-link-audit: host runtime missing " + " ".join(missing) + "; fails closed (pass --cpu-explicit for CPU)")


main()
