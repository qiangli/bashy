#!/usr/bin/env python3
"""Consumer reference contract: exact identity, signed checksums, pinned artifact SHA.

Does not install or execute the artifact. EXPECTED_SHA must come from the reviewed
consumer pin update after CI, never from an unsigned response on the same download.
"""
import hashlib
from pathlib import Path
import re
import subprocess
import sys

ASSETS = {'prima-linux-amd64','prima-linux-arm64','prima-darwin-arm64','prima-darwin-amd64'}

def validate_inputs(tag, asset, sha):
    if not re.fullmatch(r'bashy-bashy-prima-0\.1\.0-[1-9][0-9]*',tag):
        raise ValueError('unexpected release tag')
    if asset not in ASSETS: raise ValueError('unsupported asset')
    if not re.fullmatch('[0-9a-f]{64}',sha): raise ValueError('full pinned SHA-256 required')

def verify(directory, tag, asset, expected_sha):
    validate_inputs(tag,asset,expected_sha)
    identity=f'https://github.com/qiangli/prima.cpp/.github/workflows/bashy-build.yml@refs/tags/{tag}'
    for name in ['SHA256SUMS',asset]:
        path=directory/name
        if path.is_symlink() or (directory/(name+'.sigstore.json')).is_symlink():
            raise ValueError('refuse symlink')
        subprocess.run(['cosign','verify-blob','--bundle',str(directory/(name+'.sigstore.json')),
                        '--certificate-identity',identity,
                        '--certificate-oidc-issuer','https://token.actions.githubusercontent.com',
                        str(path)],check=True)
    entries={}
    for line in (directory/'SHA256SUMS').read_text().splitlines():
        m=re.fullmatch(r'([0-9a-f]{64})  ([A-Za-z0-9_.-]+)',line)
        if not m or m[2] in entries: raise ValueError('malformed or duplicate checksum entry')
        entries[m[2]]=m[1]
    digest=hashlib.sha256((directory/asset).read_bytes()).hexdigest()
    if entries.get(asset)!=expected_sha or digest!=expected_sha:
        raise ValueError('signed manifest, consumer pin and asset SHA must agree')
    print(f'PASS: {asset} sha256={digest} identity={identity}')

if __name__=='__main__':
    verify(Path(sys.argv[1]),*sys.argv[2:])
