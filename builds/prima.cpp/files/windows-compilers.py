#!/usr/bin/env python3
"""Discover a native MinGW clang pair targeting x86_64-w64-windows-gnu.

CI run 38462299969 failed here: the only clang on PATH was
C:\\Program Files\\LLVM\\bin\\clang.exe, which targets x86_64-pc-windows-msvc.
That compiler must be refused, not used: MSVC-target objects cannot link the
MinGW static build. This probe therefore demands the gnu triple, MinGW
target macros and no _MSC_VER, and refuses to install or cross-compile.
"""
import json
import os
from pathlib import Path
import shutil
import subprocess

TARGET = 'x86_64-w64-windows-gnu'
# Pinned provisioner, kept in sync with builds/prima.cpp/setup.yml.
SETUP_ACTION = 'msys2/setup-msys2@ec48f7c5447b3140e2b088413ae3a55687bccb6e'  # v2.33.0
MSYSTEM = 'CLANG64'
# Explicit install set, kept in sync with builds/prima.cpp/setup.yml.
PACKAGES = ('mingw-w64-clang-x86_64-clang', 'mingw-w64-clang-x86_64-compiler-rt',
            'mingw-w64-clang-x86_64-lld', 'mingw-w64-clang-x86_64-cmake',
            'mingw-w64-clang-x86_64-make', 'mingw-w64-clang-x86_64-ninja')


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


def find_toolchain():
    candidates = []
    cc = shutil.which('clang')
    if cc:
        candidates.append(Path(cc).parent)
    # Prefer the pinned CLANG64 environment over any other MSYS2 shell.
    candidates.append(Path('C:/msys64/clang64/bin'))
    candidates += sorted(Path('C:/msys64').glob('*/bin'))
    for directory in dict.fromkeys(candidates):
        cc, cxx = directory / 'clang.exe', directory / 'clang++.exe'
        if cc.is_file() and cxx.is_file():
            return directory
    raise ValueError('no MinGW clang pair installed (want CLANG64 from pinned setup-msys2)')


def tool_versions(directory):
    versions = {}
    for name in ('cmake', 'make', 'ninja'):
        tool = directory / (name + '.exe')
        if not tool.is_file():
            if name != 'ninja':
                raise ValueError(f'pinned toolchain is missing {tool}')
            continue
        out = subprocess.check_output([str(tool), '--version'], text=True).strip()
        versions[name] = dict(path=str(tool.resolve()), version=out.splitlines()[0] if out else '')
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
                 'compilers': records, 'tools': versions}
    Path('out').mkdir(exist_ok=True)
    Path('out/windows-compiler-identity.json').write_text(json.dumps(records, indent=2) + '\n')
    Path('out/windows-toolchain.json').write_text(json.dumps(toolchain, indent=2) + '\n')
    with open(os.environ['GITHUB_PATH'], 'a') as f:
        f.write(str(directory) + '\n')
    with open(os.environ['GITHUB_ENV'], 'a') as f:
        f.write('BASHY_WINDOWS_CC=' + records[0]['path'] + '\n')
        f.write('BASHY_WINDOWS_CXX=' + records[1]['path'] + '\n')
        for name in ('cmake', 'make'):
            tool = directory / (name + '.exe')
            if tool.is_file():
                f.write(f'BASHY_WINDOWS_{name.upper()}=' + str(tool) + '\n')
        f.write('BASHY_MINGW_ROOT=' + str(directory.parent) + '\n')
        f.write('BASHY_TOOLCHAIN_JSON=' + str(Path('out/windows-toolchain.json').resolve()) + '\n')


if __name__ == '__main__':
    main()
