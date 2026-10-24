#!/usr/bin/env python3
"""Repacked archives carry one identical signed copy of every PE file."""
import pathlib
import subprocess
import sys
import tempfile
import zipfile

script = pathlib.Path(__file__).with_name('sign-windows-release.py')
with tempfile.TemporaryDirectory() as temp:
    root = pathlib.Path(temp)
    dist, stage = root / 'dist', root / 'stage'
    dist.mkdir()
    names_before = set()
    for arch in ('amd64', 'arm64'):
        full = dist / f'bashy-windows-{arch}.zip'
        small = dist / f'bash-windows-{arch}.zip'
        raw = dist / f'outpost-v1.0.0-windows-{arch}.exe'
        raw.write_bytes(b'outpost-old')
        with zipfile.ZipFile(full, 'w') as z:
            for name in ('bashy', 'outpost', 'bash', 'sh'):
                z.writestr(f'{name}.exe', (name + '-old').encode())
            z.writestr('README.md', b'readme')
        with zipfile.ZipFile(small, 'w') as z:
            for name in ('bash', 'sh'):
                z.writestr(f'{name}.exe', (name + '-old').encode())
        names_before.update((full.name, small.name, raw.name))
    subprocess.run([sys.executable, script, 'prepare', dist, stage], check=True)
    for file in stage.rglob('*.exe'):
        file.write_bytes(file.read_bytes() + b'-signed')
    subprocess.run([sys.executable, script, 'finish', dist, stage], check=True)
    assert {f.name for f in dist.iterdir()} == names_before
    for arch in ('amd64', 'arm64'):
        with zipfile.ZipFile(dist / f'bashy-windows-{arch}.zip') as full, zipfile.ZipFile(dist / f'bash-windows-{arch}.zip') as small:
            for name in ('bash', 'sh'):
                assert full.read(name + '.exe') == small.read(name + '.exe') == (name + '-old-signed').encode()
            assert full.read('outpost.exe') == (dist / f'outpost-v1.0.0-windows-{arch}.exe').read_bytes()
            assert full.read('README.md') == b'readme'
print('Windows signing package: PASS')
