#!/usr/bin/env python3
"""Require and verify compiler inputs from every native build before auditing."""
import hashlib
import json
from pathlib import Path, PurePosixPath
import sys
import tarfile

ASSETS = ('prima-linux-amd64', 'prima-linux-arm64',
          'prima-darwin-amd64', 'prima-darwin-arm64',
          'prima-windows-amd64')

def merge(source, destination, report):
    records = []
    for asset in ASSETS:
        expected = json.loads((source / f'{asset}-inputs.json').read_text())
        hashes = {row['path']: row['sha256'] for row in expected}
        if not hashes or len(hashes) != len(expected):
            raise ValueError('empty or duplicate input manifest')
        seen = set()
        with tarfile.open(source / f'{asset}-inputs.tar.gz', 'r:gz') as archive:
            for member in archive:
                path = PurePosixPath(member.name)
                if path.is_absolute() or '..' in path.parts:
                    raise ValueError('unsafe archive path')
                if member.isdir():
                    continue
                name = str(path)
                if not member.isfile() or name not in hashes or name in seen:
                    raise ValueError('unexpected archive entry')
                data = archive.extractfile(member).read()
                if hashlib.sha256(data).hexdigest() != hashes[name]:
                    raise ValueError('compiler input hash mismatch')
                target = destination / asset / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_bytes(data)
                seen.add(name)
                records.append({'path': f'{asset}/{name}', 'sha256': hashes[name]})
        if seen != hashes.keys():
            raise ValueError('incomplete compiler input archive')
    report.write_text(json.dumps(records, indent=2) + '\n')

if __name__ == '__main__':
    merge(*map(Path, sys.argv[1:]))
