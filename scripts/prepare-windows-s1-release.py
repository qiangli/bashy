#!/usr/bin/env python3
"""Adapt the approved Windows S1 source driver for published-product testing.

Usage: prepare-windows-s1-release.py SOURCE DESTINATION
ROOT/product must contain the checksum-verified release bash.exe and bashy.exe.
Source-unit gates remain source-unit evidence; only external product invocations
measure release bytes. Ruby-only gates are excluded under operator decision D11.
"""
import pathlib
import sys


def prepare(source: str) -> str:
    replacements = {
        'run shell-build bashy go build -o bin/bash.exe ./cmd/bash':
            'mkdir -p "$ROOT/bashy/bin"\n'
            'cp "$ROOT/product/bash.exe" "$BASH_ENGINE_BIN" || exit 2',
        'run product-install bashy go build -o "$BASHY_BIN" ./cmd/bashy':
            'cp "$ROOT/product/bashy.exe" "$BASHY_BIN" || exit 2',
    }
    for old, new in replacements.items():
        if source.count(old) != 1:
            raise ValueError(f"approved driver shape changed: {old}")
        source = source.replace(old, new)
    excluded = []
    lines = []
    for line in source.splitlines():
        if line.startswith('run ') and ' bashsharp-tests ruby ' in line:
            excluded.append(line.split()[1])
            lines.append('# D11 policy exclusion (not a measured skip): ' + line)
        else:
            lines.append(line)
    if excluded != ['lower-differential', 'decorators', 'agentic']:
        raise ValueError(f"approved Ruby gate inventory changed: {excluded}")
    return '\n'.join(lines) + '\n'


if __name__ == '__main__':
    if len(sys.argv) != 3:
        raise SystemExit(__doc__)
    source, destination = map(pathlib.Path, sys.argv[1:])
    if source.resolve() == destination.resolve():
        raise SystemExit('retain the original driver; destination must differ')
    destination.write_text(prepare(source.read_text()))
