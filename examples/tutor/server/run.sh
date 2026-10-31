#!/usr/bin/env bash
# serve|join|stop — Go stdlib server (story bb7e856516c1, Sprint 547).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
verb=${1:-}
[[ $verb == serve || $verb == join || $verb == stop ]] || { echo "usage: tutor serve|join|stop" >&2; exit 2; }

cache=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/tutor
exe=$cache/tutor-server
[[ ${OS:-} == Windows_NT ]] && exe=$exe.exe
if [[ -n ${TUTOR_SERVER_BIN:-} ]]; then
  exe=$TUTOR_SERVER_BIN
elif [[ ! -x $exe || -n $(find "$here" -maxdepth 1 \( -name '*.go' -o -name go.mod \) -newer "$exe" 2>/dev/null) ]]; then
  mkdir -p "$cache"
  gocmd=(go)
  command -v go >/dev/null 2>&1 || gocmd=("${BASHY:-bashy}" go)
  (cd "$here" && "${gocmd[@]}" build -o "$exe" .) || { echo "tutor: cannot build the server (need Go)" >&2; exit 1; }
fi
export BASHY_BIN=${BASHY_BIN:-${BASHY:-bashy}}
exec "$exe" "$@"
