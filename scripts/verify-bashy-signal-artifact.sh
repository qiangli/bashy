#!/bin/sh
# Reject a Bashy artifact that cannot preserve inherited signals on its target
# or uses a diagnostic build profile that is not eligible for release.
# Used by GoReleaser post-build hooks before the publish phase. Remove a bad
# staging artifact so a failed release cannot leave it looking publishable.
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: $0 ARTIFACT" >&2
	exit 2
fi

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
artifact=$1
case "$artifact" in
	/*) ;;
	*) artifact=$PWD/$artifact ;;
esac
if (cd "$root" && go run ./tools/elfaudit --bashy-signal "$artifact" && go run ./tools/releaseeligibility "$artifact"); then
	exit 0
fi
rm -f -- "$artifact"
echo "verify-bashy-signal-artifact: rejected and removed $artifact" >&2
exit 1
