#!/bin/bash
# Generate an SPDX 2.3 JSON SBOM for a built release binary and enforce the permissive-only license gate.
# Usage: ./scripts/generate-release-sbom.sh <binary-path> [target-os] [target-arch] [outdir]
set -euo pipefail

if [[ $# -lt 1 ]]; then
  echo "usage: $0 <binary-path> [target-os] [target-arch] [outdir]" >&2
  exit 2
fi

binary=$1
target_os=${2:-}
target_arch=${3:-}
outdir=${4:-dist/sbom}

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

if [[ ! -f "$binary" ]]; then
  echo "error: binary not found: $binary" >&2
  exit 1
fi

mkdir -p "$outdir"

# Determine output file name
name=$(basename "$binary")
if [[ -n "$target_os" && -n "$target_arch" ]]; then
  outname="bashy-$target_os-$target_arch.spdx.json"
else
  outname="$name.spdx.json"
fi

outfile="$outdir/$outname"

flags=(--binary "$binary" --gate -o "$outfile")
if [[ -n "$target_os" ]]; then
  flags+=(--os "$target_os")
fi
if [[ -n "$target_arch" ]]; then
  flags+=(--arch "$target_arch")
fi

echo "==> Generating SPDX 2.3 SBOM for $binary ($target_os/$target_arch)..."

# Run through built bin/bashy if available, or go run ./cmd/bashy
if [[ -x bin/bashy ]]; then
  bin/bashy release sbom "${flags[@]}"
elif [[ -x bin/bashy.exe ]]; then
  bin/bashy.exe release sbom "${flags[@]}"
else
  go run ./cmd/bashy release sbom "${flags[@]}"
fi

echo "==> Release SBOM verified and written to $outfile"
