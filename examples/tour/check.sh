#!/bin/sh
# Bash# tour gate (e2e): run every section on a bashy binary (default: the
# one on PATH — the INSTALLED binary) and diff stdout+stderr against its
# pinned transcript. Exit 0 = every REQUIRED section matched. Optional
# sections report SKIP with the reason and never count as a pass.
#
#   ./check.sh                 # the bashy on PATH
#   ./check.sh /path/to/bashy  # another binary
#   BASHSHARP_FLAG=--bashpp ./check.sh   # a binary older than the rename
set -u
here=$(cd "$(dirname "$0")" && pwd)
bashy=${1:-bashy}
flag=${BASHSHARP_FLAG:---bashsharp}
noflag=--no-${flag#--}
rc=0
pass=0; fail=0; skip=0

diffit() { # name expected-file actual-text
    if [ "$3" = "$(cat "$2")" ]; then
        echo "tour: PASS $1"; pass=$((pass+1))
    else
        echo "tour: FAIL $1 ($bashy $flag)" >&2
        printf '%s\n' "$3" | diff -u "$2" - >&2 || true
        fail=$((fail+1)); rc=1
    fi
}
run() { # section
    diffit "$1" "$here/$1.expected" "$("$bashy" "$flag" "$here/$1.bsh" 2>&1 || true)"
}

# 00 runs twice: with the dialect on and off, same transcript both times.
run 00-bash-is-bash
diffit "00-bash-is-bash (dialect off)" "$here/00-bash-is-bash.expected" \
    "$("$bashy" "$noflag" "$here/00-bash-is-bash.bsh" 2>&1 || true)"
for s in 01-go-values 02-kwargs-defaults 03-decorators 04-enums 05-readonly \
         06-contracts 07-agentic-yield; do run "$s"; done

# 08 is fed to `check`, not run; the checker must exit 2 AND name the deref.
out=$("$bashy" check "$flag" "$here/08-null-safety.bsh" 2>&1); crc=$?
if [ "$crc" -eq 2 ]; then diffit 08-null-safety "$here/08-null-safety.expected" "$out"
else echo "tour: FAIL 08-null-safety (check exit $crc, expected 2)" >&2; fail=$((fail+1)); rc=1; fi

# 09 needs a Python.
if command -v python3 >/dev/null 2>&1; then
    run 09-python-island
else
    echo "tour: SKIP 09-python-island (no python3 on PATH)"; skip=$((skip+1))
fi

# 10 runs directly always. Lowering needs go >= 1.27 on PATH, and the emitted
# Go imports the shellrt runtime from the sh engine module, so building it
# needs a checkout of github.com/qiangli/sh: BASHSHARP_SH_ROOT=/path/to/sh.
run 10-transpile
gover=$(GOTOOLCHAIN=local go version 2>/dev/null | sed -n 's/^go version go\([0-9]*\.[0-9]*\).*/\1/p')
if [ -z "$gover" ] || [ "$(printf '%s\n1.27\n' "$gover" | sort -V | head -1)" != 1.27 ]; then
    echo "tour: SKIP 10-transpile lowered (needs go >= 1.27 on PATH; found '${gover:-none}')"; skip=$((skip+1))
elif [ -z "${BASHSHARP_SH_ROOT:-}" ] || [ ! -d "$BASHSHARP_SH_ROOT/lower/shellrt" ]; then
    echo "tour: SKIP 10-transpile lowered (set BASHSHARP_SH_ROOT to a github.com/qiangli/sh checkout: the lowered program imports its shellrt runtime)"; skip=$((skip+1))
else
    tmp=$(mktemp -d); trap 'rm -rf "$tmp"' EXIT
    printf 'module tour\n\ngo 1.27\n\nrequire mvdan.cc/sh/v3 v3.0.0\nreplace mvdan.cc/sh/v3 => %s\n' "$BASHSHARP_SH_ROOT" >"$tmp/go.mod"
    if (cd "$tmp" && GOFLAGS=-mod=mod "$bashy" transpile "$flag" "$here/10-transpile.bsh" -o "$tmp/t.go" >"$tmp/log" 2>&1 \
         && GOWORK=off GOFLAGS=-mod=mod go build -o program t.go >>"$tmp/log" 2>&1) \
       && out=$("$tmp/program" 2>&1) && [ "$out" = "$(cat "$here/10-transpile.expected")" ]; then
        echo "tour: PASS 10-transpile lowered (built as Go, same transcript)"; pass=$((pass+1))
    else
        echo "tour: FAIL 10-transpile lowered" >&2; cat "$tmp/log" >&2; printf '%s\n' "${out-}" >&2
        fail=$((fail+1)); rc=1
    fi
fi

echo "tour: $pass passed, $fail failed, $skip skipped"
exit $rc
