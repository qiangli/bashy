#!/usr/bin/env python3
"""Discover a native MinGW-target clang pair on the hosted Windows image.

Does not install or cross-compile: the windows-2025 runner must already carry
a clang whose default target is x86_64-w64-mingw32 (MinGW Makefiles generator
needs a GNU-driver clang, not clang-cl/MSVC). Fails closed with the list of
rejected candidates so CI shows what the image actually provided.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess

WANT_MACHINE = "x86_64-w64-mingw32"


def probe(cc, cxx):
    records = []
    for language, path in (("c", cc), ("c++", cxx)):
        path = Path(path).resolve()
        target = subprocess.check_output(
            [str(path), "-dumpmachine"], text=True, timeout=60).strip()
        version = subprocess.check_output(
            [str(path), "--version"], text=True, timeout=60).strip()
        if target != WANT_MACHINE or not version:
            raise ValueError(
                f"not a native MinGW amd64 compiler: {path}: {target!r}")
        lang_flag = "-x" + ("c++" if language == "c++" else "c")
        macros = subprocess.check_output(
            [str(path), "-dM", "-E", lang_flag, "-"],
            input="", text=True, timeout=60)
        if "#define __clang__ " not in macros:
            raise ValueError(f"not clang: {path}")
        if "#define __MINGW64__ " not in macros:
            raise ValueError(f"not a MinGW target: {path}")
        if ("#define __x86_64__ " not in macros
                or "#define __SIZEOF_POINTER__ 8" not in macros):
            raise ValueError(f"not Windows amd64: {path}")
        if language == "c++" and "#define __cplusplus " not in macros:
            raise ValueError(f"not a C++ compiler: {path}")
        records.append(dict(path=str(path), target=target,
                            version=version.splitlines()[0]))
    if Path(records[0]["path"]).parent != Path(records[1]["path"]).parent:
        raise ValueError("clang/clang++ installation mismatch")
    return records


def candidates():
    seen = []
    path_cc = shutil.which("clang")
    if path_cc:
        seen.append(Path(path_cc).parent)
    for root in (Path("C:/msys64/mingw64/bin"),
                 Path("C:/Program Files/LLVM/bin")):
        seen.append(root)
    return list(dict.fromkeys(seen))


def main():
    errors = []
    for directory in candidates():
        cc, cxx = directory / "clang.exe", directory / "clang++.exe"
        if not cc.is_file() or not cxx.is_file():
            cc, cxx = directory / "clang", directory / "clang++"
        try:
            records = probe(cc, cxx)
        except (OSError, ValueError, subprocess.CalledProcessError) as exc:
            errors.append(f"{directory}: {exc}")
            continue
        dump = (shutil.which("objdump") or shutil.which("llvm-objdump"))
        if not dump:
            errors.append(f"{directory}: no objdump/llvm-objdump for import evidence")
            continue
        Path("out").mkdir(exist_ok=True)
        Path("out/windows-compiler-identity.json").write_text(
            json.dumps(records, indent=2) + "\n")
        with open(os.environ["GITHUB_PATH"], "a") as f:
            f.write(str(directory.resolve()) + "\n")
        with open(os.environ["GITHUB_ENV"], "a") as f:
            f.write("BASHY_WINDOWS_CC=" + records[0]["path"] + "\n")
            f.write("BASHY_WINDOWS_CXX=" + records[1]["path"] + "\n")
        print("windows compilers: " + records[0]["path"])
        return
    raise SystemExit("no verified native MinGW clang pair: " + "; ".join(errors))


if __name__ == "__main__":
    main()
