#!/usr/bin/env python3
"""Discover a native MinGW clang pair targeting x86_64-w64-windows-gnu.

CI run 38465115827 failed here: msys2/setup-msys2 had installed CLANG64
(clang, cmake, make, ninja), but the probe picked the first clang on PATH,
C:\\Program Files\\LLVM\\bin\\clang.exe, which targets
x86_64-pc-windows-msvc. That compiler must never be selected: MSVC-target
objects cannot link the MinGW static build. This probe therefore looks ONLY
in <msys2-location>/clang64/bin (wired from the setup-msys2 step output via
the MSYS2_LOCATION environment variable, defaulting to C:/msys64), demands
the gnu triple, MinGW target macros and no _MSC_VER, and refuses to install
or cross-compile.

CI run 38466935835 failed one step later in this same probe: the MSYS2
package mingw-w64-clang-x86_64-make installs mingw32-make.exe, NOT
make.exe, so requiring clang64/bin/make.exe can never pass. The pinned
build tool is therefore clang64/bin/mingw32-make.exe, handed to CMake as
CMAKE_MAKE_PROGRAM under the MinGW Makefiles generator. The same audit
added the packages that own the other Windows-leg names the later steps
need: mingw-w64-clang-x86_64-llvm-openmp owns clang64/bin/libomp.dll (the
shipped OpenMP runtime; it is not a runtime dependency of the clang
package, so it must be installed explicitly) and
mingw-w64-clang-x86_64-llvm-tools owns clang64/bin/llvm-objdump.exe
(the DLL-import evidence tool).
"""
import json
import os
from pathlib import Path
import subprocess

TARGET = 'x86_64-w64-windows-gnu'
# Pinned provisioner, kept in sync with builds/prima.cpp/setup.yml.
SETUP_ACTION = 'msys2/setup-msys2@ec48f7c5447b3140e2b088413ae3a55687bccb6e'  # v2.33.0
MSYSTEM = 'CLANG64'
# Explicit install set, kept in sync with builds/prima.cpp/setup.yml.
PACKAGES = ('mingw-w64-clang-x86_64-clang', 'mingw-w64-clang-x86_64-compiler-rt',
            'mingw-w64-clang-x86_64-lld', 'mingw-w64-clang-x86_64-cmake',
            'mingw-w64-clang-x86_64-make', 'mingw-w64-clang-x86_64-ninja',
            'mingw-w64-clang-x86_64-llvm-openmp',
            'mingw-w64-clang-x86_64-llvm-tools')
# Pinned CMake generator and the exact executable names the pinned packages
# install into clang64/bin. The make package installs mingw32-make.exe
# (never make.exe); the llvm-tools package installs llvm-objdump.exe.
GENERATOR = 'MinGW Makefiles'
TOOLS = {'cmake': 'cmake.exe', 'make': 'mingw32-make.exe',
         'ninja': 'ninja.exe', 'objdump': 'llvm-objdump.exe'}
OMP_DLL = 'libomp.dll'


def probe(cc, cxx):
    records = []
    for language, path in (('c', cc), ('c++', cxx)):
        path = Path(path).resolve()
        target = subprocess.check_output([str(path), '-dumpmachine'], text=True).strip()
        version = subprocess.check_output([str(path), '--version'], text=True).strip()
        if target != TARGET or not version:
            raise ValueError(f'not native MinGW clang {TARGET}: {path}: {target}')
        macros = subprocess.check_output([str(path), '-dM', '-E', '-x', language, '-'],
                                         input='', text=True)
        if '#define __clang__ ' not in macros:
            raise ValueError(f'not clang: {path}')
        if '#define _MSC_VER ' in macros:
            raise ValueError(f'MSVC-target clang refused (want {TARGET}): {path}')
        if '#define __MINGW32__ ' not in macros and '#define __MINGW64__ ' not in macros:
            raise ValueError(f'not MinGW clang: {path}')
        if '#define __x86_64__ ' not in macros or '#define __SIZEOF_POINTER__ 8' not in macros:
            raise ValueError(f'not native Windows amd64 clang: {path}: target macros')
        if language == 'c++' and '#define __cplusplus ' not in macros:
            raise ValueError(f'not a C++ compiler: {path}')
        records.append(dict(path=str(path), target=target,
                            version=version.splitlines()[0], release=version.splitlines()[0]))
    if records[0]['release'] != records[1]['release'] or Path(records[0]['path']).parent != Path(records[1]['path']).parent:
        raise ValueError('clang/clang++ installation mismatch')
    return records


# Environment variable carrying the setup-msys2 step's msys2-location
# output (see builds/prima.cpp/setup.yml). Falls back to the default
# install root when unset (local runs outside CI).
MSYS2_LOCATION_ENV = 'MSYS2_LOCATION'
DEFAULT_MSYS2_ROOT = Path('C:/msys64')
# Pinned MSYS2 shell: only CLANG64 carries the MinGW clang packages.
CLANG64_SUBDIR = Path('clang64/bin')


def msys2_root():
    location = os.environ.get(MSYS2_LOCATION_ENV, '').strip()
    if location:
        return Path(location)
    return DEFAULT_MSYS2_ROOT


def clang64_dir():
    return msys2_root() / CLANG64_SUBDIR


def find_toolchain():
    # The pinned CLANG64 environment ONLY: never consult the process search
    # list (on the hosted runner it resolves to an x86_64-pc-windows-msvc
    # compiler) nor any other MSYS2 shell.
    directory = clang64_dir()
    cc, cxx = directory / 'clang.exe', directory / 'clang++.exe'
    if cc.is_file() and cxx.is_file():
        return directory
    raise ValueError(
        f'no MinGW clang pair installed (want {directory}/clang.exe and '
        f'{directory}/clang++.exe from pinned setup-msys2 CLANG64; '
        f'set {MSYS2_LOCATION_ENV} from steps.msys2.outputs.msys2-location)')


def tool_versions(directory):
    # Every build tool is required under its exact pinned executable name:
    # run 38466935835 proved a bare 'make.exe' assumption fails the leg,
    # and the DLL evidence and OpenMP shipping steps need their own pinned
    # names just as much. No PATH lookup: only clang64/bin counts.
    versions = {}
    for name, exe in TOOLS.items():
        tool = directory / exe
        if not tool.is_file():
            raise ValueError(f'pinned toolchain is missing {tool}')
        out = subprocess.check_output([str(tool), '--version'], text=True).strip()
        versions[name] = dict(path=str(tool.resolve()), exe=exe,
                              version=out.splitlines()[0] if out else '')
    omp = directory / OMP_DLL
    if not omp.is_file():
        raise ValueError(f'pinned toolchain is missing {omp}')
    versions['openmp_runtime'] = dict(path=str(omp.resolve()), exe=OMP_DLL)
    try:
        listing = subprocess.check_output(['pacman', '-Q', *PACKAGES],
                                          text=True, stderr=subprocess.DEVNULL).strip()
        versions['pacman'] = listing
    except (OSError, subprocess.CalledProcessError):
        versions['pacman'] = 'unavailable'
    return versions


def main():
    try:
        directory = find_toolchain()
    except ValueError as exc:
        raise SystemExit(f'no verified native MinGW clang pair: {exc}')
    try:
        records = probe(directory / 'clang.exe', directory / 'clang++.exe')
        versions = tool_versions(directory)
    except (OSError, ValueError, subprocess.CalledProcessError) as exc:
        raise SystemExit(f'no verified native MinGW clang pair: {exc}')
    directory = directory.resolve()
    toolchain = {'schema': 'bashy-prima-windows-toolchain-v1',
                 'target': TARGET, 'msystem': MSYSTEM,
                 'setup_action': SETUP_ACTION, 'packages': list(PACKAGES),
                 'generator': GENERATOR,
                 'make_program': versions['make']['path'],
                 'compilers': records, 'tools': versions}
    Path('out').mkdir(exist_ok=True)
    Path('out/windows-compiler-identity.json').write_text(json.dumps(records, indent=2) + '\n')
    Path('out/windows-toolchain.json').write_text(json.dumps(toolchain, indent=2) + '\n')
    with open(os.environ['GITHUB_PATH'], 'a') as f:
        f.write(str(directory) + '\n')
    with open(os.environ['GITHUB_ENV'], 'a') as f:
        f.write('BASHY_WINDOWS_CC=' + records[0]['path'] + '\n')
        f.write('BASHY_WINDOWS_CXX=' + records[1]['path'] + '\n')
        for name in ('cmake', 'make', 'ninja', 'objdump'):
            f.write(f'BASHY_WINDOWS_{name.upper()}=' + versions[name]['path'] + '\n')
        f.write('BASHY_MINGW_ROOT=' + str(directory.parent) + '\n')
        f.write('BASHY_TOOLCHAIN_JSON=' + str(Path('out/windows-toolchain.json').resolve()) + '\n')


if __name__ == '__main__':
    main()
