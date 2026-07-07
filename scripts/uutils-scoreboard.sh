#!/usr/bin/env bash
# uutils test-suite scoreboard — runs the MIT-licensed uutils/coreutils
# test suite (tests/by-util/*.rs via cargo) against the pure-Go
# coreutils MULTICALL binary built from ../coreutils. That binary serves
# the same tool registry bashy mounts in-process, and the suite's
# supported external-binary override (UUTESTS_BINARY_PATH — designed for
# e.g. WASI binaries) accepts it directly because the invocation shape
# (`coreutils <util> args…`) is identical.
#
# INFO scoreboard (like yash/zsh) — never a 0/1 gate: plenty of uutils
# cases assert uutils-specific diagnostics or extensions beyond the GNU
# manual, so 100% is not the target; the trend is the signal.
#
# Requires: cargo, and the local uutils clone (a gitignored reference
# checkout) at ../coreutils/reference/uutils-coreutils.
set -u
cd "$(dirname "$0")/.."
ROOT=$PWD
OUT=${1:-/tmp/uutils-scoreboard}
UU=${UUTILS:-$ROOT/../coreutils/reference/uutils-coreutils}
THREADS=${THREADS:-8}
mkdir -p "$OUT"

[ -d "$UU/tests/by-util" ] || {
  echo "uutils clone not found at $UU (reference/ is gitignored — clone github.com/uutils/coreutils there)" >&2
  exit 2
}
command -v cargo >/dev/null 2>&1 || { echo "need cargo (rust toolchain)" >&2; exit 2; }

# SUT override: point at a prebuilt multicall binary (e.g. one scp'd to
# a bench box) instead of building ../coreutils here.
if [ -n "${SUT:-}" ]; then
  [ -x "$SUT" ] || { echo "SUT not executable: $SUT" >&2; exit 2; }
else
  echo "building ../coreutils multicall…" >&2
  ( cd ../coreutils && go build -trimpath -o bin/coreutils ./cmd/coreutils ) || exit 2
  SUT=$(cd ../coreutils && pwd)/bin/coreutils
fi

echo "building uutils test harness (cargo; first build is slow)…" >&2
( cd "$UU" && cargo test --features unix --test tests --no-run >/dev/null 2>&1 ) || {
  echo "cargo test harness build failed" >&2
  exit 2
}

RAW="$OUT/run.txt"
echo "running uutils suite against $SUT…" >&2
( cd "$UU" && UUTESTS_BINARY_PATH="$SUT" cargo test --features unix --test tests -- --test-threads="$THREADS" ) >"$RAW" 2>&1 &
SUITE_PID=$!

# Memory watchdog: a conformance run must never take the host down (a
# runaway tool once did, via an unguarded huge allocation — see shuf's
# host-OOM guard). If the suite's process group exceeds MEMCAP_MB of
# resident memory, kill it and report, rather than swapping the host to
# death.
MEMCAP_MB=${MEMCAP_MB:-16384}
KILLED=0
while kill -0 "$SUITE_PID" 2>/dev/null; do
  RSS_MB=$(ps -ax -o pgid=,rss= | awk -v pg="$(ps -o pgid= -p $SUITE_PID | tr -d ' ')" \
    '$1 == pg { s += $2 } END { print int(s/1024) }')
  if [ "${RSS_MB:-0}" -gt "$MEMCAP_MB" ]; then
    echo "watchdog: suite exceeded ${MEMCAP_MB}MB RSS (${RSS_MB}MB) — killing" >&2
    kill -TERM -- -"$(ps -o pgid= -p $SUITE_PID | tr -d ' ')" 2>/dev/null
    sleep 2
    kill -KILL -- -"$(ps -o pgid= -p $SUITE_PID | tr -d ' ')" 2>/dev/null
    KILLED=1
    break
  fi
  sleep 2
done
wait "$SUITE_PID" 2>/dev/null
[ "$KILLED" = 1 ] && echo "watchdog: run was killed; scoreboard below is PARTIAL" >&2

# Per-util scoreboard from the per-test verdict lines.
awk '
  /^test test_[a-z0-9_]+::/ {
    split($2, a, "::"); mod = a[1]
    verdict = $NF
    if (verdict == "ok") pass[mod]++
    else if (verdict == "FAILED") fail[mod]++
    else if (verdict == "ignored" || $(NF-1) == "ignored,") ign[mod]++
    total[mod]++
  }
  END {
    tp = tf = ti = 0
    for (m in total) { tp += pass[m]; tf += fail[m]; ti += ign[m] }
    printf "=== uutils suite scoreboard (features=unix) ===\n"
    printf "total: %d pass / %d fail / %d ignored  (%d%% of %d run)\n", tp, tf, ti, (tp+tf ? 100*tp/(tp+tf) : 0), tp+tf
    printf "--- weakest utils (fail desc) ---\n"
    for (m in total) if (fail[m] > 0) printf "%4d fail / %4d  %s\n", fail[m], pass[m]+fail[m], m | "sort -rn | head -20"
  }
' "$RAW"
grep '^test test_' "$RAW" | grep 'FAILED$' | awk '{print $2}' | sort > "$OUT/failures.txt"
echo "full run: $RAW ; failing cases: $OUT/failures.txt" >&2
grep -E '^test result:' "$RAW" | tail -1
