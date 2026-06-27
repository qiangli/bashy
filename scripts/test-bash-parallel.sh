#!/usr/bin/env bash
# test-bash-parallel.sh — run the bash 5.3 fixture suite in parallel groups.
#
# The 86 fixtures are independent (each runs bin/bash in its own TMPDIR with a
# per-process scratch file), so they parallelize cleanly. This splits them
# round-robin into JOBS groups and runs `make test-bash-run TESTS="<group>"`
# for each group concurrently against the already-built bin/bash, then
# aggregates the per-group Results lines into one total.
#
# Invoked by `make test-bash-parallel` (which builds bin/bash + helpers first).
# JOBS defaults to the CPU count; on a big box, `make test-bash-parallel JOBS=20`.
# Round-robin (not chunked) spreads the few slow fixtures across groups so wall
# time is balanced. Heads-up: oversubscribing cores raises the chance a heavy
# fixture brushes the per-test timeout — keep JOBS at/under the core count.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd); cd "$HERE" || exit 2

ncpu() { sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4; }
JOBS=${JOBS:-$(ncpu)}; [ "$JOBS" -ge 1 ] 2>/dev/null || JOBS=$(ncpu)
TDIR=${BASH_TESTS_DIR:-external/bash-5.3/tests}

# Fixture names = run-* minus run-all/run-minimal (mirrors the Makefile loop).
# Portable array fill (no mapfile — macOS ships bash 3.2).
FIX=()
while IFS= read -r x; do [ -n "$x" ] && FIX+=("$x"); done < <(
  cd "$TDIR" && for r in run-*; do
    case "$r" in run-all|run-minimal) continue;; esac; echo "${r#run-}"; done | sort)
n=${#FIX[@]}
[ "$n" -gt 0 ] || { echo "test-bash-parallel: no fixtures found in $TDIR" >&2; exit 2; }
[ "$JOBS" -gt "$n" ] && JOBS=$n

OUT=$(mktemp -d 2>/dev/null || echo /tmp/tbp.$$); trap 'rm -rf "$OUT"' EXIT
echo "test-bash-parallel: $n fixtures across $JOBS parallel groups (round-robin)"

# Round-robin assign fixtures to per-group files (avoids bash-3.2 set -u array
# quirks), then launch a `test-bash-run` per group against the built bin/bash.
i=0
for name in "${FIX[@]}"; do echo "$name" >>"$OUT/grp.$(( i % JOBS ))"; i=$(( i + 1 )); done

start=$(date +%s 2>/dev/null || echo 0)
for g in $(seq 0 $((JOBS - 1))); do
  [ -f "$OUT/grp.$g" ] || continue
  grp=$(tr '\n' ' ' <"$OUT/grp.$g")
  ( make --no-print-directory test-bash-run \
       TESTS="$grp" BASH_TEST_SKIP="${BASH_TEST_SKIP:-}" \
       >"$OUT/g$g.out" 2>&1 ) &
done
wait
end=$(date +%s 2>/dev/null || echo 0)

# Aggregate the per-group "Results: P passed, F failed, S skipped, T timed out".
P=0 F=0 S=0 T=0
for f in "$OUT"/g*.out; do
  line=$(grep -E '^Results:' "$f" | tail -1)
  set -- $(printf '%s\n' "$line" | grep -oE '[0-9]+' | head -4)
  P=$((P + ${1:-0})); F=$((F + ${2:-0})); S=$((S + ${3:-0})); T=$((T + ${4:-0}))
done

# Surface any non-PASS fixtures (FAIL/TIME) so the run is actionable.
fails=$(grep -hE '^  (FAIL|TIME)  ' "$OUT"/g*.out 2>/dev/null | sort -u)
[ -n "$fails" ] && { echo; echo "non-PASS fixtures:"; echo "$fails"; }

echo
echo "Results: $P passed, $F failed, $S skipped, $T timed out  (${JOBS} groups, $((end - start))s)"
[ "$F" -eq 0 ] && [ "$T" -eq 0 ]
