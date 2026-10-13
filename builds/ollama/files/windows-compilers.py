#!/usr/bin/env python3
"""Discover an installed native MinGW GCC pair; do not install or cross-compile."""
import json
import os
from pathlib import Path
import shutil
import subprocess


def probe(cc, cxx):
    records = []
    for language, path in (("c", cc), ("c++", cxx)):
        path = Path(path).resolve()
        target = subprocess.check_output([str(path), '-dumpmachine'], text=True).strip()
        version = subprocess.check_output([str(path), '--version'], text=True).strip()
        release = subprocess.check_output([str(path), '-dumpfullversion'], text=True).strip()
        if target != 'x86_64-w64-mingw32' or not version or not release:
            raise ValueError(f'not native Windows amd64 GCC: {path}: {target}')
        macros = subprocess.check_output([str(path), '-dM', '-E', '-x', language, '-'], input='', text=True)
        if '#define __GNUC__ ' not in macros or '#define __clang__ ' in macros or '#define __MINGW64__ ' not in macros:
            raise ValueError(f'not MinGW GCC: {path}')
        if '#define __x86_64__ ' not in macros or '#define __SIZEOF_POINTER__ 8' not in macros:
            raise ValueError(f'not native Windows amd64 GCC: {path}: target macros')
        if language == 'c++' and '#define __cplusplus ' not in macros:
            raise ValueError(f'not a C++ compiler: {path}')
        records.append(dict(path=str(path), target=target, version=version, release=release))
    if records[0]['release'] != records[1]['release'] or Path(records[0]['path']).parent != Path(records[1]['path']).parent:
        raise ValueError('GCC/G++ installation mismatch')
    return records


def main():
    candidates = []
    cc = shutil.which('gcc')
    if cc:
        candidates.append(Path(cc).parent)
    candidates += sorted(Path('C:/msys64').glob('*/bin'))
    errors = []
    for directory in dict.fromkeys(candidates):
        cc, cxx = directory / 'gcc.exe', directory / 'g++.exe'
        try:
            records = probe(cc, cxx)
        except (OSError, ValueError, subprocess.CalledProcessError) as exc:
            errors.append(str(exc))
            continue
        Path('out').mkdir(exist_ok=True)
        Path('out/windows-compiler-identity.json').write_text(json.dumps(records, indent=2) + '\n')
        with open(os.environ['GITHUB_PATH'], 'a') as f:
            f.write(str(directory.resolve()) + '\n')
        with open(os.environ['GITHUB_ENV'], 'a') as f:
            f.write('BASHY_WINDOWS_CC=' + records[0]['path'] + '\n')
            f.write('BASHY_WINDOWS_CXX=' + records[1]['path'] + '\n')
            f.write('BASHY_MINGW_ROOT=' + str(directory.resolve().parent) + '\n')
        return
    raise SystemExit('no verified native GCC/G++ pair: ' + '; '.join(errors))


if __name__ == '__main__':
    main()
