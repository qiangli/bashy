#!/bin/sh
# Installed-product smoke for the rod (Sprint 238, S238.4): a language or
# toolchain bashy has never heard of, added with a runner and nothing else —
# an interpreter (awk, runner REGISTERED in a private ring), a compiler (zig,
# inline Bash# func builder over the toolchain bashy provisions) and a config
# (env, inline shell function) — plus the refusal of an unregistered program.
set -eu

root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd -P)
tmp=$(mktemp -d "${TMPDIR:-/tmp}/bashy-runners.XXXXXX")
trap 'rm -rf "$tmp"' EXIT HUP INT TERM

fail() {
	echo "runner-examples-smoke: FAIL: $*" >&2
	exit 1
}

bashy=${BASHY_BIN:-}
[ -n "$bashy" ] || bashy=$(command -v bashy 2>/dev/null || true)
[ -n "$bashy" ] && [ -x "$bashy" ] || fail "set BASHY_BIN to the installed bashy executable"
bashy_dir=$(CDPATH= cd -- "$(dirname "$bashy")" && pwd -P)
bashy=$bashy_dir/$(basename "$bashy")
case "$bashy" in "$root"/*) fail "repo-local binary is not installed-product evidence: $bashy" ;; esac

export BASHY_HINTS=off
export BASHY_COMMANDS_DIR="$tmp/ring"
mkdir -p "$BASHY_COMMANDS_DIR"

expect() { # <label> <output-file> <substring>
	grep -q -- "$3" "$2" || { cat "$2" >&2; fail "$1: expected $3"; }
}

# The registered path: register once, name it from a script that declares nothing.
"$bashy" commands register awk-runner --set exec.0="$root/examples/runners/awk-runner.sh" --set effects.0=exec >/dev/null 2>&1 || fail "commands register awk-runner"
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/awk.bsh" >"$tmp/awk.out" 2>&1) || { cat "$tmp/awk.out" >&2; fail "awk.bsh"; }
expect awk "$tmp/awk.out" "awk: ok"
expect awk "$tmp/awk.out" "this script has [0-9]* words"
echo "awk: ok (interpreter, registered runner)"

# The inline compiler: build into the fence's directory, run, guard-capped.
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/zig.bsh" >"$tmp/zig.out" 2>&1) || { cat "$tmp/zig.out" >&2; fail "zig.bsh"; }
expect zig "$tmp/zig.out" "built app"
expect zig "$tmp/zig.out" "hello from zig, built by a fenced builder"
expect zig "$tmp/zig.out" "guarded run status=126"
echo "zig: ok (compiler, inline func builder)"

# The inline config: show, apply denied under a read cap, then applied.
(cd "$tmp" && "$bashy" --bashsharp "$root/examples/runners/env.bsh" >"$tmp/env.out" 2>&1) || { cat "$tmp/env.out" >&2; fail "env.bsh"; }
expect env "$tmp/env.out" "REGION=eu-west"
expect env "$tmp/env.out" "rehearsal: status=126"
expect env "$tmp/env.out" "applied -> rendered.env"
echo "env: ok (config, inline shell runner)"

# A program that is not registered is never a runner.
printf '~~~cfg as c !not-a-registered-tool\nx\n~~~\nc.run()\n' >"$tmp/refuse.bsh"
if "$bashy" --bashsharp "$tmp/refuse.bsh" >"$tmp/refuse.out" 2>&1; then fail "unregistered runner accepted"; fi
expect refuse "$tmp/refuse.out" "PATH is never consulted"
echo "refusal: ok (unregistered program)"
[ ! -e "$tmp/rendered.env" ] && [ ! -e "$tmp/app" ] || fail "an example wrote into the caller's directory"
echo "runner-examples-smoke: OK"
