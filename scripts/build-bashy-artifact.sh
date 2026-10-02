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
tmp=$(mktemp "$out.pending.XXXXXX")
trap 'rm -f "$tmp"' EXIT HUP INT TERM

if [ -n "${BASHY_EXE:-}" ]; then
	if [ -n "$tags" ]; then
		"$BASHY_EXE" go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$tmp" ./cmd/bashy
	else
		"$BASHY_EXE" go build -trimpath -ldflags "$ldflags" -o "$tmp" ./cmd/bashy
	fi
	(unset GOOS GOARCH CGO_ENABLED; "$BASHY_EXE" go run ./tools/elfaudit --bashy-signal "$tmp")
	(unset GOOS GOARCH CGO_ENABLED; "$BASHY_EXE" go run ./tools/releaseeligibility "$tmp")
else
	if [ -n "$tags" ]; then
		go build -trimpath -tags "$tags" -ldflags "$ldflags" -o "$tmp" ./cmd/bashy
	else
		go build -trimpath -ldflags "$ldflags" -o "$tmp" ./cmd/bashy
	fi
	(unset GOOS GOARCH CGO_ENABLED; go run ./tools/elfaudit --bashy-signal "$tmp")
	(unset GOOS GOARCH CGO_ENABLED; go run ./tools/releaseeligibility "$tmp")
fi

mv -f "$tmp" "$out"
rm -f "$out.real"
