#!/bin/sh
# make-shell-var-reducer.sh — public synthetic reducer for the Profile B
# make TP97/TP99 divergence (sanitized: no licensed suite content).
#
# What it measures: how the recipe shell that make(1) spawns handles the
# SHELL variable, across the three inherited states — absent, explicitly
# empty, explicit nonempty — under an IDENTICAL provider PATH whose only
# difference is which binary `sh` resolves to (a GNU Bash oracle vs the
# bashy drop-in). GNU Bash 5.3 startup binds a non-exported SHELL (the
# login shell) when none was imported; imported values — including the
# empty string — are preserved verbatim with their export attribute.
#
# Usage:
#   scripts/make-shell-var-reducer.sh [GNU_BASH [SUT_BASH]]
# Defaults: GNU_BASH from $GNU_BASH or `bash` on PATH (must be GNU bash 5.x),
# SUT_BASH = ./bin/bash relative to the repo root.
#
# Exit status: 0 when every matrix row matches between the two providers,
# 1 on the first divergence (printed), 2 on setup failure.
set -u

repo_root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
gnu=${1:-${GNU_BASH:-bash}}
sut=${2:-$repo_root/bin/bash}

command -v "$gnu" >/dev/null 2>&1 || { echo "reducer: GNU bash oracle '$gnu' not found" >&2; exit 2; }
[ -x "$sut" ] || { echo "reducer: SUT '$sut' not found (run: make build-bash)" >&2; exit 2; }
case $("$gnu" --version 2>/dev/null | head -n1) in
*"GNU bash, version 5."*) ;;
*) echo "reducer: '$gnu' is not GNU bash 5.x — pass a real oracle" >&2; exit 2 ;;
esac
command -v make >/dev/null 2>&1 || { echo "reducer: make not found" >&2; exit 2; }

work=$(mktemp -d) || exit 2
trap 'rm -rf "$work"' EXIT INT TERM

mkdir "$work/prov-gnu" "$work/prov-sut"
ln -s "$(command -v "$gnu")" "$work/prov-gnu/sh"
ln -s "$sut" "$work/prov-sut/sh"

# The recipe probes exactly what a conformance recipe can observe:
# is $SHELL set in the recipe shell, and does a child process inherit it.
# (The synthesized value is host-specific — the login shell — so set-ness
# and child-env visibility are the portable assertions.)
cat > "$work/Makefile" <<'EOF'
SHELL = sh
probe:
	@if [ "$${SHELL+set}" = set ]; then echo "var=set"; else echo "var=unset"; fi
	@if /usr/bin/env | grep -q '^SHELL='; then echo "childenv=$$(/usr/bin/env | grep '^SHELL=')"; else echo "childenv=absent"; fi
EOF

run_matrix() { # $1 = provider dir
	for state in absent empty nonempty; do
		case $state in
		absent)   set -- /usr/bin/env -u SHELL ;;
		empty)    set -- /usr/bin/env SHELL= ;;
		nonempty) set -- /usr/bin/env SHELL=/oracle/nonempty-shell ;;
		esac
		echo "== SHELL=$state =="
		(cd "$work" && "$@" PATH="$1:/usr/bin:/bin" make -s probe 2>&1)
	done
}

run_matrix "$work/prov-gnu" > "$work/gnu.out"
run_matrix "$work/prov-sut" > "$work/sut.out"

if ! diff -u "$work/gnu.out" "$work/sut.out"; then
	echo "reducer: DIVERGENCE between GNU sh and SUT sh under make (see diff above)" >&2
	exit 1
fi
echo "reducer: PASS — GNU and SUT recipe shells agree on all SHELL states"
