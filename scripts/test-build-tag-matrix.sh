#!/bin/sh
# The two host build layers are INDEPENDENT switches, so they make four
# combinations — and only the two the Makefile targets exercise (lean via
# `make build`, both via `make build-host`) ever get compiled by a human.
#
# `BASHY_ENGINES=1` alone is a profile the Makefile documents as supported and
# nothing ever built: it broke in 2026-07 when a !bashy_obs file started calling
# helpers that lived behind !bashy_engines, and stayed broken for six weeks
# because no lane compiles that corner. Type-check all four so a tag pairing
# cannot rot unobserved again.
#
# Type-check only (`go build -o /dev/null`): this proves the tag combinations
# resolve, which is the failure mode. Linking each engine build would cost
# minutes and prove nothing further about tags.
set -eu
repo=$(CDPATH= cd -P "$(dirname "$0")/.." && pwd)
cd "$repo"

fail() { echo "test-build-tag-matrix: $*" >&2; exit 1; }

# Engine tags are unix-only (cgo, btrfs/MLX), so on Windows only the tagless
# and obs-only columns are meaningful. One combination per line: a tag set is a
# SPACE-SEPARATED list, so splitting the matrix on whitespace would silently
# test the wrong thing rather than fail.
if [ "$(go env GOOS)" = windows ]; then
	combos=$(printf '%s\n' '' 'bashy_obs')
else
	combos=$(printf '%s\n' '' 'bashy_engines' 'bashy_obs' 'bashy_engines bashy_obs')
fi

printf '%s\n' "$combos" | while IFS= read -r tags; do
	label=${tags:-lean}
	go build -tags "$tags" -o /dev/null ./cmd/bashy ||
		fail "cmd/bashy does not build with tags [$label]"
done || exit 1
echo 'test-build-tag-matrix: PASS'
