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

# Exercise the data dispatch with no fork or server.
help_out=$("$runner" --bashsharp "$here/whiteboard.bsh" --help)
[[ "$help_out" == *'whiteboard boards'* ]]
[[ "$help_out" == *'whiteboard new [NAME]'* ]]
[[ "$help_out" == *'whiteboard export ID'* ]]
[[ "$help_out" == *'whiteboard share ID'* ]]

if "$runner" --bashsharp "$here/whiteboard.bsh" show >"$tmp/show.out" 2>&1; then
  echo 'show succeeded without an ID' >&2
  exit 1
fi
grep -q 'invalid_argument' "$tmp/show.out"

if "$runner" --bashsharp "$here/whiteboard.bsh" export demo --format png >"$tmp/png.out" 2>&1; then
  echo 'png export was not refused' >&2
  exit 1
fi
grep -q 'no server API' "$tmp/png.out"

export WHITEBOARD_URL=http://127.0.0.1:22752/whiteboard
result=$("$runner" --bashsharp "$here/whiteboard.bsh" new demo --note 'hello' --dry-run --json)
printf '%s' "$result" | python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["dryRun"] is True; assert x["body"] == {"action":"add","board":"demo","body":"hello"}'
result=$("$runner" --bashsharp "$here/whiteboard.bsh" rm demo --dry-run --json)
printf '%s' "$result" | python3 -c 'import json,sys; x=json.load(sys.stdin); assert x["dryRun"] is True; assert x["body"] == {"action":"remove","id":"demo"}'

# No server is listening on the dry-run port; a real read fails closed.
if "$runner" --bashsharp "$here/whiteboard.bsh" show demo --json >"$tmp/offline.json" 2>&1; then
  echo 'show succeeded without a server' >&2
  exit 1
fi
python3 -c 'import json,sys; assert json.load(open(sys.argv[1]))["code"] == "unavailable"' "$tmp/offline.json"

PYTHONDONTWRITEBYTECODE=1 python3 -m unittest discover -s "$here" -p '*_test.py'
echo 'whiteboard data command checks passed'
