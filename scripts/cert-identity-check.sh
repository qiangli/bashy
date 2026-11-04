#!/bin/sh
# Compare a recorded certification closure with a Git revision (default HEAD).
set -eu
if [ "$#" -lt 1 ] || [ "$#" -gt 2 ]; then
    echo "usage: $0 <manifest.json> [rev]" >&2
    exit 2
fi
manifest=$(cd "$(dirname "$1")" && pwd)/$(basename "$1")
cd "$(dirname "$0")/.."
exec python3 scripts/cert-closure.py --check "$manifest" --rev "${2:-HEAD}"
