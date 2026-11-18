#!/usr/bin/env python3
"""Compare one authenticated GitHub asset listing with reviewed local bytes."""
import hashlib
import json
import pathlib
import sys

remote = json.loads(pathlib.Path(sys.argv[1]).read_text())
directory = pathlib.Path(sys.argv[2])
assets = {p.name: p for p in directory.iterdir() if p.is_file()}
if len(remote) != len(assets) or {a['name'] for a in remote} != set(assets):
    sys.exit('remote draft asset set differs from reviewed local bytes')
for entry in remote:
    path = assets[entry['name']]
    expected = 'sha256:' + hashlib.sha256(path.read_bytes()).hexdigest()
    if entry.get('state') != 'uploaded' or entry.get('size') != path.stat().st_size or entry.get('digest') != expected:
        sys.exit('remote draft asset changed or lacks a server digest: ' + path.name)
print('remote draft asset digests: PASS')
