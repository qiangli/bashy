#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home"
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

result=$("$runner" --bashsharp "$here/chat.bsh" status demo)
[[ $result == *'site: demo not installed'* ]]
[[ $result == *'site: demo stopped'* ]]

if "$runner" --bashsharp "$here/chat.bsh" new --template static demo >"$tmp/template.out" 2>&1; then
  echo 'unsupported template succeeded' >&2
  exit 1
fi
grep -q -- '--template chat' "$tmp/template.out"

if "$runner" --bashsharp "$here/chat.bsh" new --template chat 'Bad Name' >"$tmp/name.out" 2>&1; then
  echo 'invalid name succeeded' >&2
  exit 1
fi
grep -q 'NAME must begin with a letter' "$tmp/name.out"

"$runner" --bashsharp "$here/chat.bsh" --help | grep -q 'OPENAI_COMPATIBLE_BASE_URL'
"$runner" --bashsharp "$here/chat.bsh" --help | grep -q 'export'

# Dispatch via site.bsh parent wrapper
result_parent=$("$runner" --bashsharp "$here/../site.bsh" status demo)
[[ $result_parent == *'site: demo not installed'* ]]

if "$runner" --bashsharp "$here/../site.bsh" export demo "$tmp/out" >"$tmp/export.out" 2>&1; then
  echo 'export uninstalled succeeded' >&2
  exit 1
fi
grep -q 'run site new --template chat demo first' "$tmp/export.out"

echo 'site command unit checks passed'
