#!/usr/bin/env python3
"""Collect actual compiler input closure, with file hashes; exclude host SDKs."""
import hashlib
import json
import os
from pathlib import Path
import re
import shlex
import shutil
import sys

# A depfile rule is `targets: prerequisites`. Targets may carry a Windows
# drive prefix (C:/...), whose colon must not split the rule; the rule colon
# is the first colon outside a leading drive prefix.
_DRIVE = re.compile(r'^[A-Za-z]:')

def rule_prerequisites(text):
    body = text.replace('\\\n', ' ').replace('\\\r\n', ' ')
    if _DRIVE.match(body):
        head, sep, tail = body[2:].partition(':')
        assert sep, 'malformed depfile rule'
        return tail
    return body.split(':', 1)[1]

def dependency_paths(build):
    paths = set()
    patterns = ['*.o.d', '*.obj.d']
    found = False
    for pattern in patterns:
        for dep in build.rglob(pattern):
            found = True
            for item in shlex.split(rule_prerequisites(dep.read_text(errors='replace'))):
                p = Path(item).resolve()
                paths.add(p)
    assert found, 'no compiler dependency evidence'
    return paths

def collect(src, build, dest, report):
    src = src.resolve()
    build = build.resolve()
    paths = {p for p in dependency_paths(build)
             if (p.is_relative_to(src) or p.is_relative_to(build)) and p.is_file()}
    assert paths, 'no compiler dependency evidence'
    # Audit arm64-only Metal code even in the Linux audit job.
    for pattern in ['ggml/src/*metal*', 'ggml/include/*metal*']:
        paths.update(p for p in src.glob(pattern) if p.is_file())
    paths.update([src/'LICENSE', src/'deps/nng/LICENSE.txt'])
    records = []
    for p in sorted(paths):
        rel = p.relative_to(src) if p.is_relative_to(src) else Path('_generated')/p.relative_to(build)
        target = dest/rel
        target.parent.mkdir(parents=True, exist_ok=True)
        shutil.copyfile(p, target)
        records.append({'path': str(rel), 'sha256': hashlib.sha256(p.read_bytes()).hexdigest()})
    # The pinned Windows toolchain record (see windows-compilers.py) joins
    # the closure so its package versions land in the inputs tarball and SBOM.
    toolchain = os.environ.get('BASHY_TOOLCHAIN_JSON')
    if toolchain:
        blob = Path(toolchain).read_bytes()
        (dest/'TOOLCHAIN.json').write_bytes(blob)
        records.append({'path': 'TOOLCHAIN.json', 'sha256': hashlib.sha256(blob).hexdigest()})
    Path(report).write_text(json.dumps(records, indent=2)+'\n')

if __name__ == '__main__':
    collect(*map(Path, sys.argv[1:]))
