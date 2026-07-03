#!/usr/bin/env bash
# e2e startup-performance guard.
#
# Measures bashy's package-init() budget via GODEBUG=inittrace=1 and asserts
# that the container/LLM ENGINES are NOT linked onto the default (lean) hot
# path. The engines (podman + ollama/mlx) are cgo-heavy: linking them roughly
# doubles both binary size (68 MB -> 146 MB) and cold spawn (~8 ms -> ~17 ms on
# Apple Silicon). They are opt-in via `-tags bashy_engines` for host nodes that
# run containers/inference locally; a lean worker delegates to a host node over
# the mesh. Keeping them off the default link graph is the single biggest
# startup lever (#6), so this is the regression guard that catches an engine
# import silently creeping back into the lean worker — the same role
# scripts/verify-builtins.sh plays for builtin classification.
#
# NOT denied: the tree-sitter code-intel (`list-symbols`/`search-symbols`/…).
# It costs ~0.34 ms of init and stays LINKED on purpose — the readily-available
# invariant says `bashy <cmd>` must just work with no second step, and 0.34 ms
# is not worth an exec/provisioning detour. For agents that fire thousands of
# calls, the warm `bashy serve` session (see internal/agentos/session) amortizes
# even that to ~0 by paying init once. See dhnt/docs/bashy-startup-performance.md.
#
# Usage:  BASHY=bin/bashy scripts/perf-startup.sh
# Env:    BASH_BIN            (pure `bash` drop-in, for the spawn-ratio note)
#         STARTUP_BUDGET_MS   (informational init threshold; default 30)
set -uo pipefail

BASHY="${BASHY:-$(cd "$(dirname "$0")/.." && pwd)/bin/bashy}"
BASH_BIN="${BASH_BIN:-$(cd "$(dirname "$0")/.." && pwd)/bin/bash}"
BUDGET_MS="${STARTUP_BUDGET_MS:-30}"

# Engine package PREFIXES that must stay OFF the default (lean) hot path. Any
# init-trace package whose name starts with one of these means a cgo engine was
# re-linked into the always-loaded worker. Matched as prefixes so any
# subpackage trips the guard.
DENY=(
  "go.podman.io/"                        # podman container engine (image/storage libs)
  "github.com/ollama/ollama/"            # ollama LLM engine (incl. x/mlxrunner/mlx)
  "github.com/containers/"               # podman's container libraries
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
pkglist="$(awk '/^init /{print $2}' "$trace")"
for p in "${DENY[@]}"; do
  if printf '%s\n' $pkglist | grep -q "^${p}"; then
    hit="$(printf '%s\n' $pkglist | grep "^${p}" | head -1)"
    echo "FAIL: container/LLM engine on the default hot path: $hit (prefix $p)" >&2
    echo "  -> the lean worker must not link engines; build them with -tags bashy_engines only." >&2
    fail=1
  fi
done

# Informational: lean-vs-pure-bash spawn ratio. The <<1.5x hot-path budget is
# only reachable via the warm session for a fat multi-call binary; a cold spawn
# of the lean binary lands around 2-2.5x on Apple Silicon. This is a soft signal
# to watch the trend, not a hard gate.
if [ -x "$BASH_BIN" ]; then
  spawn() { local b="$1" n=40 i; local t0 t1
    t0=$(date +%s.%N); for ((i=0;i<n;i++)); do "$b" -c true; done; t1=$(date +%s.%N)
    awk -v a="$t0" -v b="$t1" -v n="$n" 'BEGIN{printf "%.2f", (b-a)/n*1000}'
  }
  sb="$(spawn "$BASH_BIN")"; sy="$(spawn "$BASHY")"
  awk -v sb="$sb" -v sy="$sy" 'BEGIN{
    printf "spawn: bash %.2f ms, bashy %.2f ms, ratio %.2fx (warm session -> ~0)\n", sb, sy, (sb>0? sy/sb : 0)
  }'
fi

# Informational init budget note (machine-dependent; the DENY list is the hard gate).
awk -v t="$total" -v thr="$BUDGET_MS" 'BEGIN{
  if (t+0 > thr+0) printf "NOTE: init budget %.2f ms exceeds soft threshold %s ms\n", t, thr
}'

rm -f "$trace"
if [ "$fail" -ne 0 ]; then
  echo "perf-startup: FAIL — a container/LLM engine is linked into the lean hot path." >&2
  exit 1
fi
echo "perf-startup: OK (engines off the lean hot path)"
