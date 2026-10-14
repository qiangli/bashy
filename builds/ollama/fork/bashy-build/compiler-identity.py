#!/usr/bin/env python3
"""Record the exact native compiler selection before cgo wrappers are installed."""
import hashlib
import importlib.util
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys

# Revalidate the exact pair at build time, not only the earlier PATH discovery.
if os.environ.get('GOOS') == 'windows':
    probe_path = Path(__file__).with_name('windows-compilers.py')
    if not probe_path.is_file():
        # Source tree: the probe is shared via builds/_shared and only sits
        # beside this file in the rendered fork.
        probe_path = Path(__file__).resolve().parent.parent / '_shared' / 'windows-compilers.py'
    spec = importlib.util.spec_from_file_location('windows', probe_path)
    windows = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(windows)
    records = windows.probe(Path(sys.argv[2]), Path(sys.argv[3]))
    for row in records:
        row['sha256'] = hashlib.sha256(Path(row['path']).read_bytes()).hexdigest()
    Path(sys.argv[1]).write_text(json.dumps(records, indent=2) + '\n')
    sys.exit(0)

records = []
for name in sys.argv[2:]:
    path = shutil.which(name)
    if not path:
        raise SystemExit('compiler not found: ' + name)
    records.append({'path': str(Path(path).absolute()),
                    'resolved_path': str(Path(path).resolve()),
                    'sha256': hashlib.sha256(Path(path).read_bytes()).hexdigest(),
                    'version': subprocess.check_output([path, '--version'], text=True),
                    'target': subprocess.check_output([path, '-dumpmachine'], text=True).strip()})
Path(sys.argv[1]).write_text(json.dumps(records, indent=2) + '\n')
