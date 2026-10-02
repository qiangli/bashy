#!/bin/sh
# Reject a Linux Bashy artifact whose ELF cannot preserve inherited signals.
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
if (cd "$root" && go run ./tools/elfaudit --bashy-signal "$artifact"); then
	exit 0
fi
rm -f -- "$artifact"
echo "verify-bashy-signal-artifact: rejected and removed $artifact" >&2
exit 1
