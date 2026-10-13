"""Exact-byte adjudications of ScanCode 32.5.0 findings from runs 38420930156
and 38484129698 (rework 11: NNG Windows platform files, same MIT-header rule).

No additional license is allowed. A changed pin, file, expression, or license
requires fresh review. Raw ScanCode findings remain in prima-licenses.json.
"""
import hashlib
import json
from pathlib import Path, PurePosixPath
import re

ROOT = Path(__file__).resolve().parent
ASSETS = {'prima-linux-amd64', 'prima-linux-arm64',
          'prima-darwin-amd64', 'prima-darwin-arm64',
          'prima-windows-amd64'}
ALLOWED = {'mit', 'bsd-new', 'bsd-simplified', 'bsd-original',
           'apache-2.0', 'isc', 'public-domain'}


def audit_licenses(scan, inputs, tree, inventory):
    reviewed = json.loads((ROOT/'license-review.json').read_text())
    hashes = {f['path']: f['sha256'] for f in inputs}
    observed = [f['path'].removeprefix('audit-inputs/')
                for f in scan['files'] if f.get('type') == 'file']
    if (not hashes or len(hashes) != len(inputs) or
            len(observed) != len(set(observed)) or set(observed) != set(hashes)):
        raise ValueError('ScanCode must cover every compiler input, on all five platforms')
    if {p.split('/')[0] for p in hashes} != ASSETS:
        raise ValueError('compiler inputs must include all five platforms')
    data = {}
    for path, digest in hashes.items():
        parts = PurePosixPath(path)
        if parts.is_absolute() or '..' in parts.parts or str(parts) != path:
            raise ValueError('unsafe compiler input path')
        data[path] = (tree/path).read_bytes()
        if hashlib.sha256(data[path]).hexdigest() != digest:
            raise ValueError('compiler input hash mismatch')

    def licensed(asset, name, digest):
        return hashes.get(f'{asset}/{name}') == digest

    nng = [d for d in inventory['linked'] if d['name'] == 'nng']
    pins_match = (inventory['source']['commit'] == reviewed['source_commit'] and
                  len(nng) == 1 and nng[0]['commit'] == reviewed['nng_commit'])
    errors, corrections = [], []
    for path in hashes:
        if path.split('/', 1)[1].startswith('deps/nng/src/supplemental/sha1/'):
            errors.append(f'excluded SHA1 compiler input: {path}')
    for f in scan['files']:
        if f.get('scan_errors'):
            errors.append(f"scan error: {f['path']}")
        path = f['path'].removeprefix('audit-inputs/')
        for detection in f.get('license_detections', []):
            expression = detection['license_expression']
            ids = set(re.findall(r'[a-z][a-z0-9.+-]*', expression)) - {'and', 'or', 'with'}
            if ids and ids <= ALLOWED:
                continue
            correction = None
            if path in hashes and pins_match:
                asset, name = path.split('/', 1)
                body = data[path]
                if (expression == 'mit AND cpal-1.0' and
                        hashes[path] == reviewed['nng_files'].get(name) and
                        licensed(asset, 'deps/nng/LICENSE.txt', reviewed['nng_license_sha256']) and
                        b'MIT License' in body and
                        not re.search(rb'cpal|common public attribution|mozilla|\bmpl\b|freeware public|\bfpl\b', body, re.I)):
                    correction = 'pinned NNG MIT header; no CPAL terms in reviewed bytes'
                if (expression == 'mit AND mpl-2.0' and name == 'common/zmq_nng_shim.hpp' and
                        hashes[path] == reviewed['shim_sha256'] and
                        licensed(asset, 'LICENSE', reviewed['source_license_sha256']) and
                        body.count(b'MPL-2.0') == 1 and
                        b'MIT-licensed nng (libnng) instead of MPL-2.0 ZeroMQ.' in body):
                    correction = 'pinned first-party MIT shim; MPL occurs only in replacement description'
            if correction:
                corrections.append({'path': path, 'sha256': hashes[path],
                                    'detected': expression, 'reviewed': 'mit', 'reason': correction})
            else:
                errors.append(f"unreviewed license {expression}: {f['path']}")
    return errors, corrections
