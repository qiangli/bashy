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

# Exercise the registered script's data dispatch with no fork, credential or server.
export NOTES_URL=http://127.0.0.1:22749/notes
unset NOTES_TOKEN
result=$("$runner" --bashsharp "$here/notes.bsh" set demo --content 'hello' --visibility private --dry-run --json)
printf '%s' "$result" | python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["dryRun"] is True; assert x["body"] == {"memo":{"name":"memos/demo","content":"hello","visibility":"PRIVATE"},"updateMask":"content,visibility"}; assert "Authorization" not in x'
if "$runner" --bashsharp "$here/notes.bsh" show demo --json >"$tmp/denied.json"; then
  echo 'show succeeded without a local token' >&2
  exit 1
fi
python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["code"] == "unauthenticated"' "$tmp/denied.json"
PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$here" -p '*_test.py' -v
echo 'notes data command checks passed'
