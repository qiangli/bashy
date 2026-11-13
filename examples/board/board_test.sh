#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home"
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

test -f "$here/board-console/main.go.tmpl"
test ! -e "$here/board-console/main.go"
test -f "$here/board-console/placeholder.go"
grep -q 'alias:@sentry/node=' "$here/board.bsh"
grep -q '"@sentry/react": path.resolve' "$here/board-shape.patch"
grep -q 'function captureReactException' "$here/sentry-stub.ts"
grep -q 'function browserTracingIntegration' "$here/sentry-stub.ts"
grep -q 'function replayIntegration' "$here/sentry-stub.ts"

result=$("$runner" --bashsharp "$here/board.bsh" status)
[[ "$result" == *'board: not installed'* ]]
[[ "$result" == *'board: console stopped'* ]]
if "$runner" --bashsharp "$here/board.bsh" start >"$tmp/start.out" 2>&1; then
  echo 'start succeeded without an install' >&2
  exit 1
fi
grep -q 'run board install first' "$tmp/start.out"
if "$runner" --bashsharp "$here/board.bsh" unexpected >"$tmp/usage.out" 2>&1; then
  echo 'unknown verb succeeded' >&2
  exit 1
fi
grep -q 'board install|start|stop|status|source' "$tmp/usage.out"
echo 'board command unit checks passed'
