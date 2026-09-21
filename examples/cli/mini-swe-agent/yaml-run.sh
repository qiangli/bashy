#!/bin/sh
# YAML entrypoint for the bounded mini-swe-agent example.
#
# This is the second runnable entrypoint (main.bsh is the shell one). Parsing is
# DECLARATIVELY OWNED: the frozen generic ycode compiler validates and renders
# profile.yaml and is the strict parse gate for every invocation. Because ycode's
# frozen dispatch cannot launch a mini loop, a run that parses cleanly returns
# exit 4 ("unsupported by this CLI contract"); this bridge treats that exit 4 as
# "parsed OK — now run the LOCAL loop" and execs the same harness/ CLI that
# main.bsh runs. A real usage error is exit 2 and is propagated verbatim.
#
# This is the documented example-local bridge: no shared Go/schema change, no
# second product-specific dispatch branch — just ycode-as-parser + local loop.
set -eu

here=$(CDPATH= cd -- "$(dirname -- "$0")" && pwd)
profile="$here/profile.yaml"
harness="$here/harness"

: "${YCODE_BIN:?set YCODE_BIN to the frozen ycode compiler/renderer}"
[ -x "$YCODE_BIN" ] || { printf 'not executable: %s\n' "$YCODE_BIN" >&2; exit 1; }
if [ -z "${BASHY_BIN:-}" ]; then
    BASHY_BIN=$(command -v bashy || true)
fi
export BASHY_BIN
python_bin="${PYTHON_BIN:-python3}"

# Honor help wherever it appears, including after flags such as
# `-t TASK --help`; ycode owns rendering for those requests.
for arg in "$@"; do
    case "$arg" in
        -h|--help) exec "$YCODE_BIN" --file "$profile" --help ;;
    esac
done

verb="${1:-}"
case "$verb" in
    help | -h | --help | version | validate | completion | schema)
        exec "$YCODE_BIN" --file "$profile" "$@"
        ;;
esac

# Strict parse gate.
set +e
gate_err=$("$YCODE_BIN" --file "$profile" "$@" 2>&1 >/dev/null)
gate_rc=$?
set -e
case "$gate_rc" in
    0) exit 0 ;;                                       # a declarative op handled it
    4) : ;;                                            # bridge point: run the loop
    *) [ -n "$gate_err" ] && printf '%s\n' "$gate_err" >&2; exit "$gate_rc" ;;
esac

exec env PYTHONPATH="$harness" "$python_bin" -m minisweagent_bounded.cli "$@"
