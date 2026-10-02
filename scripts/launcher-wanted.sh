#!/bin/sh
# Prints 1 when the lean bash/sh drop-in needs its native pre-Go signal
# launcher (linux or darwin AND a C compiler on PATH), else 0. One-file
# cmd/bashy uses its own in-executable signal capture and never calls this.
# Used by the Makefile and dag.md's lean shell build task.
goos=$(go env GOOS 2>/dev/null || ${BASHY:-bashy} go env GOOS 2>/dev/null)
case "$goos" in
linux|darwin)
	if command -v cc >/dev/null 2>&1; then echo 1; else
		echo "${1:-build}: no C compiler (cc) on PATH — plain Go binary without the native signal launcher (the release-archive form)" >&2
		echo 0
	fi ;;
*) echo 0 ;;
esac
