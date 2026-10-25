#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
DATA=${BRAINSTORM_DATA:-${XDG_DATA_HOME:-$HOME/.local/share}/bashy/brainstorm}
CACHE=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/brainstorm
case ${1:-serve} in
serve)
  mkdir -p "$CACHE"
  (cd "$HERE/server" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$CACHE/brainstorm" .)
  exec "$CACHE/brainstorm" -data "$DATA" -addr "${BRAINSTORM_ADDR:-0.0.0.0:18091}"
  ;;
test) cd "$HERE/server"; GOWORK=off go test -race ./... ;;
*) echo 'Usage: bashy brainstorm.sh [serve|test]' >&2; exit 2 ;;
esac
