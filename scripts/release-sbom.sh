#!/bin/sh
# One command for the release SBOM + license gate over the build matrix:
#   lean bashy (cmd/bashy), engine build (the SHIPPED host configuration:
#   -tags bashy_engines,bashy_host,fb_archives; bashy_host keeps Podman and its
#   MPL/cgo graph out of the core release), and outpost (cmd/outpost). Usage: scripts/release-sbom.sh [outdir]
# Output: <outdir>/{bashy,bashy-engines,outpost} + <outdir>/sbom/{*.spdx.json,manifest.json}
# Fails on non-permissive/unknown license, or SBOM/binary digest drift.
# Set SKIP_ENGINES=1 where the engine build's native deps are unavailable.
set -eu
OUT=${1:-dist/sbom-matrix}
mkdir -p "$OUT"
go run ./tools/licensegate
ENGINE_TAGS=${ENGINE_TAGS:-bashy_engines,bashy_host,fb_archives}
go run ./tools/licensegate -tags "$ENGINE_TAGS" ./cmd/bashy
go build -o "$OUT/bashy" ./cmd/bashy
if [ -z "${SKIP_ENGINES:-}" ]; then
	go build -tags "$ENGINE_TAGS" -o "$OUT/bashy-engines" ./cmd/bashy
fi
go build -o "$OUT/outpost" github.com/qiangli/outpost/cmd/outpost
go run ./cmd/bashy release sbom --dist "$OUT" --inventory docs/license-inventory.md
go run ./cmd/bashy release sbom --dist "$OUT" --verify
echo "manifest: $OUT/sbom/manifest.json"
