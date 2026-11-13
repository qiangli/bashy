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

# Exercise the registered script's data dispatch with no fork, key or server.
export TABLE_URL=http://127.0.0.1:22750/table
unset TABLE_TOKEN
result=$("$runner" --bashsharp "$here/table.bsh" records set d1 T --where Status=open --set Status=done --dry-run --json)
printf '%s' "$result" | python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["dryRun"] is True; assert [r["method"] for r in x["requests"]] == ["GET","PATCH"]; assert "Authorization" not in x'
if "$runner" --bashsharp "$here/table.bsh" docs list --json >"$tmp/denied.json"; then
  echo 'docs list succeeded without a local key' >&2; exit 1
fi
python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["code"] == "unauthenticated"' "$tmp/denied.json"
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$here" -p '*_test.py' -v
echo 'table data command checks passed'
