#!/bin/sh
# Bash# quickstart gate: run every example on a bashy binary (default: the
# one on PATH — the INSTALLED binary) and diff stdout+stderr against its
# pinned transcript. Exit 0 = every example matched. Used by the
# install-matrix workflow on a freshly downloaded release asset.
#
# Mode (a) and (c) examples run here (no Go toolchain needed).
# Mode (b) — transpile --standalone — requires Go and is covered by
# `make smoke-quickstart` (scripts/quickstart-smoke.sh) instead.
#
# Hosts without python3: hello_island is skipped with an informative message;
# run `bashy check --prepare hello_island.bsh` first to provision python3.
set -u
here=$(cd "$(dirname "$0")" && pwd)
bashy=${1:-bashy}
flag=${BASHSHARP_FLAG:---bashsharp}
rc=0

# Examples with no external toolchain dependency
for ex in judge decorators kwargs enums readonly hello pipeline; do
    out=$("$bashy" "$flag" "$here/$ex.bsh" 2>&1) || true
    if [ "$out" = "$(cat "$here/$ex.expected")" ]; then
        echo "quickstart: PASS $ex"
    else
        echo "quickstart: FAIL $ex ($bashy $flag)" >&2
        printf '%s\n' "$out" | diff -u "$here/$ex.expected" - >&2 || true
        rc=1
    fi
done

# Mode (c): Python island — skip if python3 not available
if command -v python3 >/dev/null 2>&1; then
    out=$("$bashy" "$flag" "$here/hello_island.bsh" 2>&1) || true
    if [ "$out" = "$(cat "$here/hello_island.expected")" ]; then
        echo "quickstart: PASS hello_island"
    else
        echo "quickstart: FAIL hello_island ($bashy $flag)" >&2
        printf '%s\n' "$out" | diff -u "$here/hello_island.expected" - >&2 || true
        rc=1
    fi
else
    echo "quickstart: SKIP hello_island (python3 not on PATH; run 'bashy check --prepare hello_island.bsh' first)"
fi

exit $rc
