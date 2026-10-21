#!/bin/sh
# One command for the release SBOM + license gate over the build matrix:
#   lean bashy (cmd/bashy), engine build (-tags bashy_engines,fb_archives),
#   and outpost (cmd/outpost). Usage: scripts/release-sbom.sh [outdir]
# Output: <outdir>/{bashy,bashy-engines,outpost} + <outdir>/sbom/{*.spdx.json,manifest.json}
# Fails on non-permissive/unknown license, or SBOM/binary digest drift.
# Set SKIP_ENGINES=1 where the engine build's native deps are unavailable.
set -eu
OUT=${1:-dist/sbom-matrix}
mkdir -p "$OUT"
go run ./tools/licensegate
go run ./tools/licensegate -tags bashy_engines,fb_archives ./cmd/bashy
go build -o "$OUT/bashy" ./cmd/bashy
if [ -z "${SKIP_ENGINES:-}" ]; then
	go build -tags bashy_engines,fb_archives -o "$OUT/bashy-engines" ./cmd/bashy
fi
go build -o "$OUT/outpost" github.com/qiangli/outpost/cmd/outpost
go run ./cmd/bashy release sbom --dist "$OUT" --inventory docs/license-inventory.md
go run ./cmd/bashy release sbom --dist "$OUT" --verify
echo "manifest: $OUT/sbom/manifest.json"
