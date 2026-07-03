#!/usr/bin/env bash
# e2e startup-performance guard.
#
# Measures bashy's package-init() budget via GODEBUG=inittrace=1 and asserts
# that known-heavy *feature* packages are NOT linked onto the default hot path
# (they must be lazy-decoded, build-tagged, or exec'd — see
# dhnt/docs/bashy-startup-performance.md). This is the regression guard that
# keeps a heavy import from silently creeping back into the lean worker, the
# same way scripts/verify-builtins.sh guards builtin classification.
#
# Usage:  BASHY=bin/bashy scripts/perf-startup.sh
# Env:    STARTUP_BUDGET_MS  (informational threshold; default 30)
set -uo pipefail

BASHY="${BASHY:-$(cd "$(dirname "$0")/.." && pwd)/bin/bashy}"
BUDGET_MS="${STARTUP_BUDGET_MS:-30}"

# Feature packages that must stay OFF the default (lean) hot path. When one of
# these appears in `bashy -c true`'s init trace it means a heavy feature was
# re-linked into the always-loaded worker. Keep this list in sync with the
# startup-performance doc.
DENY=(
  "github.com/odvcencio/gotreesitter/grammars"  # yc code-intel: gzip+gob grammar decode at init
  "github.com/odvcencio/gotreesitter"           # tree-sitter runtime (pulls encoding/gob)
)

if [ ! -x "$BASHY" ]; then
  echo "perf-startup: bashy binary not found at $BASHY (run 'make build' first)" >&2
  exit 2
fi

trace="$(mktemp)"
GODEBUG=inittrace=1 "$BASHY" -c true >/dev/null 2>"$trace" || true

total="$(awk '/^init /{s+=$5} END{printf "%.2f", s+0}' "$trace")"
pkgs="$(awk '/^init /{n++} END{print n+0}' "$trace")"

echo "bashy startup init() budget: ${total} ms across ${pkgs} packages"
echo "top offenders:"
awk '/^init /{printf "  %6s ms  %s\n", $5, $2}' "$trace" | sort -rn -k1 | head -8

fail=0
for p in "${DENY[@]}"; do
  if awk '/^init /{print $2}' "$trace" | grep -qx "$p"; then
    echo "FAIL: heavy feature package on the default hot path: $p" >&2
    fail=1
  fi
done

# Informational budget note (init clock is machine-dependent, so this is a
# soft signal, not a hard gate — the DENY list above is the hard gate).
awk -v t="$total" -v thr="$BUDGET_MS" 'BEGIN{
  if (t+0 > thr+0) printf "NOTE: init budget %.2f ms exceeds soft threshold %s ms\n", t, thr
}'

rm -f "$trace"
if [ "$fail" -ne 0 ]; then
  echo "perf-startup: FAIL — a heavy feature is linked into the lean hot path." >&2
  exit 1
fi
echo "perf-startup: OK"
