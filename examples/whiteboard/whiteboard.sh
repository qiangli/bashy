#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
# Compatibility entry point; all behavior and guidance live in whiteboard.bsh.
HERE=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
exec "${BASHY:-bashy}" --bashsharp "$HERE/whiteboard.bsh" "$@"
