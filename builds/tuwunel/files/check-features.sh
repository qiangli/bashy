#!/usr/bin/env bash
# check-features.sh SRC_DIR
#
# Fails if the bashy feature set names a telemetry/crash-reporting/profiling
# feature, or if the resolved dependency graph for that feature set (on this
# host's target) contains any Sentry / OpenTelemetry / tokio-console crate.
set -euo pipefail

here=$(cd "$(dirname "$0")" && pwd)
# shellcheck source=/dev/null
. "$here/features.env"
src=${1:?usage: check-features.sh SRC_DIR}

die() { echo "check-features: $*" >&2; exit 1; }

for f in $FEATURES; do
	for bad in $FORBIDDEN_FEATURES; do
		[[ $f != "$bad" ]] || die "forbidden feature in FEATURES: $f"
	done
done

tree=$(cd "$src" && cargo tree --locked -p tuwunel --no-default-features \
	--features "$FEATURES" -e normal,build --prefix none --format '{p}')
found=""
for crate in $FORBIDDEN_CRATES; do
	if grep -Eq "^${crate} v" <<<"$tree"; then
		found+=" $crate"
	fi
done
[[ -z $found ]] || die "forbidden crates in the dependency graph:$found"

echo "check-features: features [$FEATURES]; no telemetry crates resolved OK"
