#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home"
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

result=$("$runner" --bashsharp "$here/notes.bsh" status)
[[ "$result" == *'notes: not installed'* ]]
[[ "$result" == *'notes: console stopped'* ]]
if "$runner" --bashsharp "$here/notes.bsh" start >"$tmp/start.out" 2>&1; then
  echo 'start succeeded without an install' >&2
  exit 1
fi
grep -q 'run notes install first' "$tmp/start.out"
if "$runner" --bashsharp "$here/notes.bsh" unexpected >"$tmp/usage.out" 2>&1; then
  echo 'unknown verb succeeded' >&2
  exit 1
fi
grep -q 'notes install|start|stop|status|source' "$tmp/usage.out"
echo 'notes command unit checks passed'
