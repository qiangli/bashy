#!/usr/bin/env python3
"""Verify reviewed inputs before applying the shipped build-only overlay."""
import hashlib
import pathlib
import shutil
import subprocess
import sys

ROOT = pathlib.Path(__file__).resolve().parent
SOURCE = 'a0d55d961c8fc1b694de63bd0b2aebdb8365cf2b'
NNG = '29aff2bacc9e2cfa6dcc52155d2816582617f918'

def git(root, *args):
    return subprocess.check_output(['git', '-C', str(root), *args], text=True).strip()

def prepare(src):
    assert git(src, 'rev-parse', 'HEAD') == SOURCE, 'wrong source'
    assert not git(src, 'status', '--porcelain', '--ignored'), 'dirty source'
    assert hashlib.sha256((src/'LICENSE').read_bytes()).hexdigest() == '0ed0eea0d0f31b7f07774a7cac64f9fd20351faf51fab447e9444734448d0452'
    dep = src/'deps/nng'
    subprocess.run(['git', 'clone', '--depth', '1', '--branch', 'v1.9.0',
                    'https://github.com/nanomsg/nng', str(dep)], check=True)
    assert git(dep, 'rev-parse', 'HEAD') == NNG, 'nng tag moved'
    assert git(dep, 'hash-object', 'LICENSE.txt') == '353da18316279d3eded2dd973f21af1cce87acf8', 'nng license changed'
    # No dependency on a host fio binary (GPL); preserve documented fallback estimates.
    p = src/'common/profiler.cpp'
    text = p.read_text()
    begin = text.index('static void external_fio_impl(')
    end = text.index('\nvoid device_disk_rnd_bw(', begin)
    text = text[:begin] + '''static void external_fio_impl(float * read_bw, float * write_bw, bool, int) {
    *read_bw = 0.1f;
    *write_bw = 0.1f;
}
''' + text[end:]
    p.write_text(text)
    shutil.copyfile(ROOT/'overlay.cmake', src/'bashy-overlay.cmake')
    with (src/'CMakeLists.txt').open('a') as f:
        f.write('\ninclude(${CMAKE_CURRENT_SOURCE_DIR}/bashy-overlay.cmake)\n')
    print('verified source/nng/license pins; applied CMake wiring and fio removal')

if __name__ == '__main__':
    prepare(pathlib.Path(sys.argv[1]).resolve())
