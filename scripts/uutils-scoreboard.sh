#!/usr/bin/env bash
# uutils test-suite scoreboard --- runs the MIT-licensed uutils/coreutils
# test suite (tests/by-util/*.rs via cargo) against the pure-Go
# coreutils MULTICALL binary built from ../coreutils. That binary serves
# the same tool registry bashy mounts in-process, and the suite's
# supported external-binary override (UUTESTS_BINARY_PATH --- designed for
# e.g. WASI binaries) accepts it directly because the invocation shape
# (`coreutils <util> args---`) is identical.
#
# INFO scoreboard (like yash/zsh) --- never a 0/1 gate: plenty of uutils
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
THREADS=${THREADS:-2}
mkdir -p "$OUT"

# Never drive the foreign, adversarial suite directly on the steward host.
# Its cases include infinite devices and recursive root-equivalent operands.
# The RSS polling watchdog is not containment: several workers can allocate
# gigabytes between polls, and a recursive chmod/chgrp can mutate host-owned
# paths without consuming much memory.  The supported runner must be a
# disposable container with hard cgroup memory/PID limits and no host-root
# mount.  Keep the override awkward and explicit for harness development only.
if [ "${UUTILS_UNSAFE_HOST:-0}" != 1 ]; then
  cat >&2 <<'EOF'
uutils-scoreboard: REFUSED on the host.
The upstream suite contains OOM and recursive-root landmines. Run it only in a
disposable container with hard memory/PID limits and no host-root mount.
For isolated harness development only: UUTILS_UNSAFE_HOST=1 (still quarantines
known cases unless UUTILS_UNSAFE_LANDMINES=1 is also set).
EOF
  exit 2
fi

# HOST-SAFETY QUARANTINE
#
# These upstream tests are intentionally adversarial.  They are safe against
# uutils because uutils rejects the operands before reading/walking them, but
# the pure-Go SUT does not yet have all of those guards:
#
#   split -n 3 /dev/zero
#     enters splitChunks' in-memory whole-input path on an infinite device;
#   sort /dev/random nonexistent_file
#     reads the infinite first operand before validating the missing second;
#   chmod/chgrp -R --preserve-root PATH-THAT-RESOLVES-TO-/
#     bypasses the current string-equality root guard and walks the host root.
#
# A 2026-07-24 run spawned two >2 GiB split processes and a >1 GiB sort
# process, and also walked / through chmod/chgrp.  Never remove a skip merely
# because the suite completes once: first land the corresponding SUT guard and
# prove it with a small, isolated regression test.  UUTILS_UNSAFE_LANDMINES=1
# is deliberately loud and opt-in for a disposable, memory-capped container.
DANGEROUS_SKIPS=()
if [ "${UUTILS_UNSAFE_LANDMINES:-0}" != 1 ]; then
  DANGEROUS_SKIPS=(
    --skip test_split::test_dev_zero
    --skip test_split::test_number_by_bytes_dev_zero
    --skip test_sort::test_verifies_input_files
    --skip test_chgrp::test_preserve_root
    --skip test_chgrp::test_preserve_root_symlink
    --skip test_chgrp::test_preserve_root_symlink_cwd_root
    --skip test_chmod::test_chmod_preserve_root_with_paths_that_resolve_to_root
  )
fi

[ -d "$UU/tests/by-util" ] || {
  echo "uutils clone not found at $UU (reference/ is gitignored --- clone github.com/uutils/coreutils there)" >&2
  exit 2
}
command -v cargo >/dev/null 2>&1 || { echo "need cargo (rust toolchain)" >&2; exit 2; }

# SUT override: point at a prebuilt multicall binary (e.g. one scp'd to
# a bench box) instead of building ../coreutils here.
if [ -n "${SUT:-}" ]; then
  [ -x "$SUT" ] || { echo "SUT not executable: $SUT" >&2; exit 2; }
else
  echo "building ../coreutils multicall---" >&2
  ( cd ../coreutils && go build -trimpath -o bin/coreutils ./cmd/coreutils ) || exit 2
  SUT=$(cd ../coreutils && pwd)/bin/coreutils
fi

echo "building uutils test harness (cargo; first build is slow)---" >&2
( cd "$UU" && cargo test --features unix --test tests --no-run >/dev/null 2>&1 ) || {
  echo "cargo test harness build failed" >&2
  exit 2
}

RAW="$OUT/run.txt"
echo "running uutils suite against $SUT---" >&2
( cd "$UU" && UUTESTS_BINARY_PATH="$SUT" cargo test --features unix --test tests -- \
    --test-threads="$THREADS" "${DANGEROUS_SKIPS[@]}" ) >"$RAW" 2>&1 &
SUITE_PID=$!

# Memory watchdog: a conformance run must never take the host down (a
# runaway tool once did, via an unguarded huge allocation --- see shuf's
# host-OOM guard). If the suite's process group exceeds MEMCAP_MB of
# resident memory, kill it and report, rather than swapping the host to
# death.
MEMCAP_MB=${MEMCAP_MB:-2048}
KILLED=0
while kill -0 "$SUITE_PID" 2>/dev/null; do
  RSS_MB=$(ps -ax -o pgid=,rss= | awk -v pg="$(ps -o pgid= -p $SUITE_PID | tr -d ' ')" \
    '$1 == pg { s += $2 } END { print int(s/1024) }')
  if [ "${RSS_MB:-0}" -gt "$MEMCAP_MB" ]; then
    echo "watchdog: suite exceeded ${MEMCAP_MB}MB RSS (${RSS_MB}MB) --- killing" >&2
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
