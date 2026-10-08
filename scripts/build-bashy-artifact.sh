#!/bin/sh
# Build one cmd/bashy executable to a temporary path and publish it only after
# the Linux inherited-signal ELF contract passes. Used by dag.md build tasks;
# BASHY_EXE selects the existing Bashy front door for its managed Go toolchain.
set -eu

if [ "$#" -lt 2 ] || [ "$#" -gt 3 ]; then
	echo "usage: $0 OUTPUT LDFLAGS [TAGS]" >&2
	exit 2
fi

out=$1
ldflags=$2
tags=${3-}
# Do not use tmp: Windows inherits TMP case-insensitively, so assigning tmp
# exports the artifact FILE as Go's temporary DIRECTORY to child processes.
artifact_pending=$(mktemp "$out.pending.XXXXXX")
trap 'rm -f "$artifact_pending"' EXIT HUP INT TERM

if [ -n "${BASHY_EXE:-}" ]; then
	if [ -n "$tags" ]; then
		scripts/go-product.sh build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$artifact_pending" ./cmd/bashy
	else
		scripts/go-product.sh build -trimpath -ldflags "$ldflags" -o "$artifact_pending" ./cmd/bashy
	fi
	(unset GOOS GOARCH CGO_ENABLED; "$BASHY_EXE" go run ./tools/elfaudit --bashy-signal "$artifact_pending")
	(unset GOOS GOARCH CGO_ENABLED; "$BASHY_EXE" go run ./tools/releaseeligibility "$artifact_pending")
else
	if [ -n "$tags" ]; then
		scripts/go-product.sh build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$artifact_pending" ./cmd/bashy
	else
		scripts/go-product.sh build -trimpath -ldflags "$ldflags" -o "$artifact_pending" ./cmd/bashy
	fi
	(unset GOOS GOARCH CGO_ENABLED; go run ./tools/elfaudit --bashy-signal "$artifact_pending")
	(unset GOOS GOARCH CGO_ENABLED; go run ./tools/releaseeligibility "$artifact_pending")
fi

mv -f "$artifact_pending" "$out"
rm -f "$out.real"
