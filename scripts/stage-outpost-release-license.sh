#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
[[ -s ../outpost/LICENSE ]] || { echo 'missing pinned outpost license' >&2; exit 1; }
mkdir -p .release-assets
cp ../outpost/LICENSE .release-assets/LICENSE.outpost
