#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
# Entry point for desk calendar. All behavior and guidance live in calendar.bsh
# and the pure-Go server/CLI.
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
exec "${BASHY:-bashy}" --bashsharp "$HERE/calendar.bsh" "$@"
