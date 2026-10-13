#!/usr/bin/env python3
"""Record the exact native compiler selection before cgo wrappers are installed."""
import json
from pathlib import Path
import shutil
import subprocess
import sys

records = []
for name in sys.argv[2:]:
    path = shutil.which(name)
    if not path:
        raise SystemExit('compiler not found: ' + name)
    records.append({'path': str(Path(path).resolve()),
                    'version': subprocess.check_output([path, '--version'], text=True),
                    'target': subprocess.check_output([path, '-dumpmachine'], text=True).strip()})
Path(sys.argv[1]).write_text(json.dumps(records, indent=2) + '\n')
