#!/usr/bin/env python3
"""Fetch before isolation; verify again inside the runtime gate."""
import hashlib
import json
from pathlib import Path
import sys
import urllib.request


def verify(path, spec):
    data = Path(path).read_bytes()
    if len(data) != spec['size'] or hashlib.sha256(data).hexdigest() != spec['sha256']:
        raise RuntimeError('fixture size/SHA-256 mismatch')
    if data[:4] != b'GGUF':
        raise RuntimeError('fixture is not GGUF')


if __name__ == '__main__':
    spec = json.loads(Path(__file__).with_name('fixture.json').read_text())
    dst = Path(sys.argv[1])
    with urllib.request.urlopen(spec['url'], timeout=60) as response:
        data = response.read(spec['size'] + 1)
    dst.write_bytes(data)
    verify(dst, spec)
    print('fixture verified:', spec['sha256'])
