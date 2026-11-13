#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
export BASHY_HOME="$tmp/home"
export BASHY_HINTS=off
runner=${BASHY_BIN:-bashy}

result=$("$runner" --bashsharp "$here/site.bsh" status demo)
[[ $result == *'site: demo not installed'* ]]
[[ $result == *'site: demo stopped'* ]]

if "$runner" --bashsharp "$here/site.bsh" new --template static demo >"$tmp/template.out" 2>&1; then
  echo 'unsupported template succeeded' >&2
  exit 1
fi
grep -q -- '--template chat' "$tmp/template.out"

if "$runner" --bashsharp "$here/site.bsh" new --template chat 'Bad Name' >"$tmp/name.out" 2>&1; then
  echo 'invalid name succeeded' >&2
  exit 1
fi
grep -q 'NAME must begin with a letter' "$tmp/name.out"

"$runner" --bashsharp "$here/site.bsh" --help | grep -q 'OPENAI_COMPATIBLE_BASE_URL'
echo 'site command unit checks passed'
