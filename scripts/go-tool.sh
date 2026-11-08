#!/bin/sh
# Run a repo tool that is its own Go module (tools/NAME/go.mod), from the
# caller's working directory. `go run ./tools/NAME` no longer resolves from the
# root module, and `go -C tools/NAME run .` would move the tool's cwd and break
# repo-relative arguments (licensegate scans ., yashsuite reads manifests).
# Usage: scripts/go-tool.sh NAME [ARGS...]. Honours BASHY_EXE like go-product.sh.
set -eu
[ "$#" -ge 1 ] || { echo "usage: go-tool.sh NAME [ARGS...]" >&2; exit 2; }
name=$1
shift
# Windows callers may pass scripts\go-tool.sh; use / without external tools.
self=$0
while :; do case $self in *\\*) self=${self%%\\*}/${self#*\\} ;; *) break ;; esac; done
root=$(CDPATH= cd -- "$(dirname -- "$self")/.." && pwd)
[ -f "$root/tools/$name/go.mod" ] || { echo "go-tool: tools/$name is not a separate module" >&2; exit 2; }
gocmd() {
	if [ -n "${BASHY_EXE:-}" ]; then GOWORK=off "$BASHY_EXE" go "$@"; else GOWORK=off go "$@"; fi
}
# Not "tmp": Windows inherits TMP case-insensitively (see build-bashy-artifact.sh).
work=$(mktemp -d "${TMPDIR:-/tmp}/go-tool.XXXXXX")
trap 'rm -rf "$work"' EXIT HUP INT TERM
bin=$work/$name$(gocmd env GOEXE)
gocmd -C "$root/tools/$name" build -o "$bin" .
set +e
"$bin" "$@"
status=$?
exit "$status"
