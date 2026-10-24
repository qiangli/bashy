#!/usr/bin/env python3
"""Stage Windows PE files for the official signing Action, then package signed bytes."""
import argparse
import pathlib
import zipfile

p = argparse.ArgumentParser()
p.add_argument('phase', choices=('prepare', 'finish'))
p.add_argument('dist', type=pathlib.Path)
p.add_argument('stage', type=pathlib.Path)
a = p.parse_args()
a.stage.mkdir(parents=True, exist_ok=True)

for arch in ('amd64', 'arm64'):
    full = a.dist / f'bashy-windows-{arch}.zip'
    small = a.dist / f'bash-windows-{arch}.zip'
    raw = list(a.dist.glob(f'outpost-v*-windows-{arch}.exe'))
    if len(raw) != 1:
        raise SystemExit(f'expected one raw outpost windows/{arch} asset')
    signed = a.stage / arch
    signed.mkdir(exist_ok=True)
    with zipfile.ZipFile(full) as z:
        entries = {pathlib.PurePosixPath(n).name: n for n in z.namelist()}
        for name in ('bashy.exe', 'outpost.exe', 'bash.exe', 'sh.exe'):
            if name not in entries:
                raise SystemExit(f'{full}: missing {name}')
            if a.phase == 'prepare':
                (signed / name).write_bytes(z.read(entries[name]))
    if a.phase == 'prepare':
        if raw[0].read_bytes() != (signed / 'outpost.exe').read_bytes():
            raise SystemExit(f'{raw[0]} differs from archive outpost.exe')
        with zipfile.ZipFile(small) as z:
            for name in ('bash.exe', 'sh.exe'):
                matches = [n for n in z.namelist() if pathlib.PurePosixPath(n).name == name]
                if len(matches) != 1 or z.read(matches[0]) != (signed / name).read_bytes():
                    raise SystemExit(f'{small}: {name} differs from full archive')
        continue
    for name in ('bashy.exe', 'outpost.exe', 'bash.exe', 'sh.exe'):
        if not (signed / name).is_file():
            raise SystemExit(f'missing signed {name} for {arch}')
    for archive in (full, small):
        tmp = archive.with_suffix('.signed.tmp')
        with zipfile.ZipFile(archive) as src, zipfile.ZipFile(tmp, 'w') as dst:
            for info in src.infolist():
                name = pathlib.PurePosixPath(info.filename).name
                data = (signed / name).read_bytes() if name in ('bashy.exe', 'outpost.exe', 'bash.exe', 'sh.exe') else src.read(info)
                dst.writestr(info, data)
        tmp.replace(archive)
    raw[0].write_bytes((signed / 'outpost.exe').read_bytes())
