#!/bin/sh
# Sprint 203 gate: run the agentic-boundary contract fixture on a bashy binary
# (default: the one on PATH — the INSTALLED binary) and diff stdout+stderr
# against the pinned transcript. Exit 0 = pass.
set -eu
here=$(cd "$(dirname "$0")" && pwd)
bashy=${1:-bashy}
out=$("$bashy" --bashpp "$here/agentic-boundary.bpp" 2>&1) || true
if [ "$out" = "$(cat "$here/agentic-boundary.expected")" ]; then
    echo "contracts: PASS ($bashy)"
else
    echo "contracts: FAIL ($bashy)" >&2
    printf '%s\n' "$out" | diff -u "$here/agentic-boundary.expected" - >&2 || true
    exit 1
fi
