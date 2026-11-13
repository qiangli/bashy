#!/usr/bin/env bash
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home"
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

result=$("$runner" --bashsharp "$here/whiteboard.bsh" status)
[[ "$result" == *'whiteboard: not installed'* ]]
[[ "$result" == *'whiteboard: console stopped'* ]]

if "$runner" --bashsharp "$here/whiteboard.bsh" start >"$tmp/start.out" 2>&1; then
  echo 'start succeeded without an install' >&2
  exit 1
fi
grep -q 'run whiteboard install first' "$tmp/start.out"

if "$runner" --bashsharp "$here/whiteboard.bsh" unexpected >"$tmp/usage.out" 2>&1; then
  echo 'unknown verb succeeded' >&2
  exit 1
fi
grep -q 'whiteboard install|start|stop|status|source' "$tmp/usage.out"

if "$runner" --bashsharp "$here/whiteboard.bsh" source >"$tmp/source.out" 2>&1; then
  echo 'source succeeded without an install' >&2
  exit 1
fi

(cd "$here/server" && GOWORK=off CGO_ENABLED=0 go test -v ./...)

echo 'whiteboard command unit checks passed'
