#!/usr/bin/env python3
"""Collect actual compiler input closure, with file hashes; exclude host SDKs."""
import hashlib
import json
from pathlib import Path
import shlex
import shutil
import sys

src, build, dest, report = map(Path, sys.argv[1:])
src = src.resolve()
build = build.resolve()
paths = set()
for dep in build.rglob('*.o.d'):
    for item in shlex.split(dep.read_text().replace('\\\n', ' ').split(':',1)[1]):
        p = Path(item).resolve()
        if (p.is_relative_to(src) or p.is_relative_to(build)) and p.is_file(): paths.add(p)
assert paths, 'no compiler dependency evidence'
# Audit arm64-only Metal code even in the Linux audit job.
for pattern in ['ggml/src/*metal*', 'ggml/include/*metal*']:
    paths.update(p for p in src.glob(pattern) if p.is_file())
paths.update([src/'LICENSE', src/'deps/nng/LICENSE.txt'])
records=[]
for p in sorted(paths):
    rel=p.relative_to(src) if p.is_relative_to(src) else Path('_generated')/p.relative_to(build)
    target=dest/rel
    target.parent.mkdir(parents=True, exist_ok=True)
    shutil.copyfile(p,target)
    records.append({'path':str(rel),'sha256':hashlib.sha256(p.read_bytes()).hexdigest()})
Path(report).write_text(json.dumps(records,indent=2)+'\n')
