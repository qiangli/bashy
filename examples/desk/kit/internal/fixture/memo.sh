#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
# Fixture entry point: builds the memo server/CLI and runs the verb.
# Unlike real desk tools this fixture has no Bash# front; it exists only
# to prove the shared kit and contract test.
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
BIN=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/desk-memo/memo
mkdir -p "$(dirname "$BIN")"
(cd "$HERE" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$BIN" .)
case ${1:-serve} in
  test) (cd "$HERE" && GOWORK=off go test ./...);;
  *) exec "$BIN" "$@" ;;
esac
