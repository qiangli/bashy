#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home" BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

result=$("$runner" --bashsharp "$here/table.bsh" status)
[[ "$result" == *'table: not installed'* ]]
[[ "$result" == *'table: console stopped'* ]]
if "$runner" --bashsharp "$here/table.bsh" unexpected >"$tmp/usage.out" 2>&1; then
  echo 'unknown verb succeeded' >&2; exit 1
fi
grep -q 'table install|start|stop|status|source' "$tmp/usage.out"
if "$runner" --bashsharp "$here/table.bsh" source >"$tmp/source.out" 2>&1; then
  echo 'source succeeded without an install' >&2; exit 1
fi
(cd "$here" && GOWORK=off go test ./shape-adapter)
echo 'table command unit checks passed'
