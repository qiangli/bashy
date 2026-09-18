#!/bin/sh
# Prints 1 when the native pre-Go signal launcher should be built (linux or
# darwin AND a C compiler on PATH), else 0 — and, on linux/darwin without a
# compiler, says once on stderr that the build ships the plain Go binary: the
# same form the release archives ship, minus the launcher's job (preserving
# inherited SIGQUIT/SIGPIPE ignore dispositions). Used by the Makefile and by
# dag.md's build task so both recipes stay in step.
goos=$(go env GOOS 2>/dev/null || ${BASHY:-bashy} go env GOOS 2>/dev/null)
case "$goos" in
linux|darwin)
	if command -v cc >/dev/null 2>&1; then echo 1; else
		echo "${1:-build}: no C compiler (cc) on PATH — plain Go binary without the native signal launcher (the release-archive form)" >&2
		echo 0
	fi ;;
*) echo 0 ;;
esac
