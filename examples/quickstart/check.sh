#!/bin/sh
# Bash# quickstart gate: run every example on a bashy binary (default: the
# one on PATH — the INSTALLED binary) and diff stdout+stderr against its
# pinned transcript. Exit 0 = every example matched. Used by the
# install-matrix workflow on a freshly downloaded release asset.
set -u
here=$(cd "$(dirname "$0")" && pwd)
bashy=${1:-bashy}
flag=${BASHSHARP_FLAG:---bashsharp}
rc=0
for ex in judge decorators kwargs enums readonly; do
    out=$("$bashy" "$flag" "$here/$ex.bsh" 2>&1) || true
    if [ "$out" = "$(cat "$here/$ex.expected")" ]; then
        echo "quickstart: PASS $ex"
    else
        echo "quickstart: FAIL $ex ($bashy $flag)" >&2
        printf '%s\n' "$out" | diff -u "$here/$ex.expected" - >&2 || true
        rc=1
    fi
done
exit $rc
