#!/bin/sh
# Installed-product smoke for the Sprint 238 manifest fences: each
# examples/manifests/<tool>/build.bsh is run through `bashy awd` in a SCRATCH
# COPY of its directory, its expected line is asserted, and the copy must be
# byte-identical afterwards — the manifest, caches and outputs live under the
# fence root, never in the tree. Toolchains are what bashy provisions (cargo,
# uv, go, cmake, make, npm); none is required on the host.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-manifests.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "manifest-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in "$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;; esac

export BASHY_HINTS=off

# snapshot <dir> — names, sizes and content digests of every file.
snapshot() {
	(cd "$1" && find . -type f -o -type l | LC_ALL=C sort | while IFS= read -r f; do
		printf '%s %s\n' "$f" "$(cksum <"$f" 2>/dev/null || readlink "$f")"
	done)
}

run_one() { # <tool> <expected-substring>
	tool=$1 expect=$2
	src=$root/examples/manifests/$tool
	work=$tmp/$tool
	cp -R "$src" "$work"
	snapshot "$work" >"$tmp/$tool.before"
	if ! "$bashy" awd "$work" -- "$bashy" --bashsharp "$work/build.bsh" >"$tmp/$tool.out" 2>&1; then
		cat "$tmp/$tool.out" >&2
		fail "$tool: build.bsh failed"
	fi
	grep -q -- "$expect" "$tmp/$tool.out" || { cat "$tmp/$tool.out" >&2; fail "$tool: expected $expect"; }
	snapshot "$work" >"$tmp/$tool.after"
	cmp -s "$tmp/$tool.before" "$tmp/$tool.after" || { diff "$tmp/$tool.before" "$tmp/$tool.after" >&2 || true; fail "$tool: the directory changed"; }
	echo "$tool: ok ($expect)"
}

run_one cargo "cargo says hi from awd"
run_one pyproject "uv says hi with rich"
run_one gomod "GO FENCE + GOMOD FENCE!"
run_one cmake "test status=0"
run_one makefile "greet target"
run_one package "node script in package"
echo "manifest-examples-smoke: OK"
