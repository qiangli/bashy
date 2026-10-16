#!/usr/bin/env bash
# License gate: fail bashy/yoke/sh/outpost builds on any non-permissive license
# in Go deps or vendored code (GPL/LGPL/AGPL/MPL/CPAL/SSPL/unknown), with exact-file
# reviewed exceptions only (path + sha256 + reason, same shape as prima review).
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

exec go run ./tools/licensegate "$@"
