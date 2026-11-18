#!/usr/bin/env python3
"""Validate build receipts and emit the candidate's signing manifest line."""
import json
import pathlib
import sys

if len(sys.argv) == 4 and sys.argv[1] == 'read-note':
    import re
    body = pathlib.Path(sys.argv[2]).read_text()
    matches = re.findall(r'^Signing: darwin.signed=(true|false) windows.signed=(true|false)$', body, re.M)
    if len(matches) != 1:
        raise SystemExit('release notes need exactly one signing manifest line')
    print(dict(zip(('darwin', 'windows'), matches[0]))[sys.argv[3]])
    sys.exit(0)
root = pathlib.Path(sys.argv[1])
def flag(name):
    value = json.loads((root / f'{name}.json').read_text())['signed']
    if type(value) is not bool:
        raise SystemExit(f'{name}: signed must be a boolean')
    return value

darwin = flag('darwin-arm64')
windows = flag('windows')
print(f'Signing: darwin.signed={str(darwin).lower()} windows.signed={str(windows).lower()}')
