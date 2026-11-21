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
# Cache compiled tool binaries under bin/.tools/ keyed by Go toolchain + tool
# source fingerprint so repeated invocations (e.g. releaseeligibility during
# build-bashy-artifact.sh) avoid recompiling from scratch on every build.
cache_dir=$root/bin/.tools
mkdir -p "$cache_dir"
go_meta=$(gocmd env GOEXE GOOS GOARCH GOVERSION)
goexe=$(printf '%s\n' "$go_meta" | sed -n '1p')
go_key=$(printf '%s\n' "$go_meta" | tr '\n' '|')
tool_fp=$(
	(
		printf 'meta:%s\n' "$go_key"
		cd "$root/tools/$name" && find . -type f | LC_ALL=C sort | while IFS= read -r f; do
			printf '%s:' "$f"
			cksum "$f"
		done
	) | cksum | awk '{print $1 "-" $2}'
)
bin=$cache_dir/$name-$tool_fp$goexe
if [ ! -x "$bin" ]; then
	tmpbin=$cache_dir/.$name.tmp.$$$goexe
	trap 'rm -f "$tmpbin"' EXIT HUP INT TERM
	gocmd -C "$root/tools/$name" build -o "$tmpbin" .
	mv -f "$tmpbin" "$bin"
	trap - EXIT HUP INT TERM
fi
set +e
"$bin" "$@"
status=$?
exit "$status"
