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
# JOBS defaults to min(cpu_count, memory_slots) — see the memory gate below.
# Round-robin (not chunked) spreads the few slow fixtures across groups so wall
# time is balanced. Heads-up: oversubscribing cores raises the chance a heavy
# fixture brushes the per-test timeout — keep JOBS at/under the core count.
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd); cd "$HERE" || exit 2

ncpu() { sysctl -n hw.ncpu 2>/dev/null || nproc 2>/dev/null || echo 4; }

# Total physical memory, in KB. Darwin reports bytes via sysctl; Linux exposes
# MemTotal in KB. 0 means "unknown" and disables the gate below.
memtotal_kb() {
  if [ -r /proc/meminfo ]; then
    awk '/^MemTotal:/ { print $2; exit }' /proc/meminfo
    return
  fi
  b=$(sysctl -n hw.memsize 2>/dev/null) || b=
  if [ -n "$b" ]; then echo $(( b / 1024 )); else echo 0; fi
}

# THE MEMORY GATE.
#
# scripts/memwatch.sh caps ONE fixture's process group at BASH_TEST_MEM_KB (4 GB)
# because macOS ignores `ulimit -v` and a runaway fixture (intl/unicode1.sub's
# `printf '\U'` under a multibyte locale) balloons past 100 GB before the
# wall-clock timeout fires. That guard is per-group. Running JOBS groups
# concurrently therefore admits JOBS × BASH_TEST_MEM_KB of RSS in aggregate, and
# NO watchdog fires while every group sits under its own cap — so JOBS=$(ncpu) on
# a 12-core/24 GB host authorizes 48 GB and wedges the machine. N copies of a
# guard defeat the guard.
#
# So slots are gated by memory as often as by cores:
#     JOBS = min(cpu_count, (mem_total - reserve) / mem_per_task)
# Reserve leaves room for the OS, make(1), and the per-group `ps`/`awk` watchdog
# pollers themselves. An explicit JOBS= is CLAMPED, not honored: the failure mode
# is a dead machine, not a slow suite.
MEM_PER_JOB_KB=${BASH_TEST_MEM_KB:-4194304}
MEM_RESERVE_KB=${BASH_TEST_MEM_RESERVE_KB:-2097152}   # 2 GB headroom
JOBS=${JOBS:-$(ncpu)}; [ "$JOBS" -ge 1 ] 2>/dev/null || JOBS=$(ncpu)

TOTAL_KB=$(memtotal_kb)
if [ "$TOTAL_KB" -gt "$MEM_RESERVE_KB" ] 2>/dev/null && [ "$MEM_PER_JOB_KB" -gt 0 ] 2>/dev/null; then
  MEM_SLOTS=$(( (TOTAL_KB - MEM_RESERVE_KB) / MEM_PER_JOB_KB ))
  [ "$MEM_SLOTS" -ge 1 ] || MEM_SLOTS=1
  if [ "$JOBS" -gt "$MEM_SLOTS" ]; then
    echo "test-bash-parallel: clamping JOBS $JOBS -> $MEM_SLOTS" \
         "($(( TOTAL_KB / 1048576 ))GB RAM - $(( MEM_RESERVE_KB / 1048576 ))GB reserve" \
         "/ $(( MEM_PER_JOB_KB / 1048576 ))GB per-fixture cap)" >&2
    JOBS=$MEM_SLOTS
  fi
fi

TDIR=${BASH_TESTS_DIR:-external/bash-5.3/tests}

# The fixture tree is a GITIGNORED symlink (external/bash-5.3 -> a bash 5.3 source
# tree). Without it there is nothing to run — and an unguarded `for r in run-*`
# leaves the glob unexpanded, so the loop "runs" one fixture literally named `*`,
# scores 0 passed / 0 failed, and the final `[ $F -eq 0 ]` EXITS 0. The 86/86
# release gate would report success having executed no fixtures at all. Guard the
# directory and the glob, and fail loudly.
[ -d "$TDIR" ] || {
  echo "test-bash-parallel: fixture dir not found: $TDIR" >&2
  echo "  the bash-5.3 tests are a gitignored symlink; create it:" >&2
  echo "    mkdir -p external && ln -s /path/to/bash-5.3 external/bash-5.3" >&2
  exit 2
}

# Fixture names = run-* minus run-all/run-minimal (mirrors the Makefile loop).
# Portable array fill (no mapfile — macOS ships bash 3.2).
FIX=()
while IFS= read -r x; do [ -n "$x" ] && FIX+=("$x"); done < <(
  cd "$TDIR" && for r in run-*; do
    [ -e "$r" ] || continue          # unmatched glob stays literal; skip it
    if [ "$r" != run-all ] && [ "$r" != run-minimal ]; then
      echo "${r#run-}"
    fi
  done | sort)
n=${#FIX[@]}
[ "$n" -gt 0 ] || { echo "test-bash-parallel: no fixtures found in $TDIR" >&2; exit 2; }

# SERIAL fixtures: stateful history-expansion tests read/write $HOME/.bash_history
# and are timing-sensitive; run concurrently they flake (histexpand FAILs under
# -parallel but PASSes serial). Isolating HOME per group reduced but did not
# eliminate the race, so pin them to a serial tail — deterministic, and only ~2
# fixtures so wall time is unaffected. Keep this list minimal and evidence-based.
SERIAL_NAMES=" histexpand history "
PAR_FIX=(); SER_FIX=()
for name in "${FIX[@]}"; do
  case "$SERIAL_NAMES" in *" $name "*) SER_FIX+=("$name");; *) PAR_FIX+=("$name");; esac
done
pn=${#PAR_FIX[@]}
[ "$JOBS" -gt "$pn" ] && JOBS=$pn
[ "$JOBS" -ge 1 ] || JOBS=1

OUT=$(mktemp -d 2>/dev/null || echo /tmp/tbp.$$); trap 'rm -rf "$OUT"' EXIT
echo "test-bash-parallel: $n fixtures ($pn across $JOBS parallel groups, ${#SER_FIX[@]} serial)"

# Round-robin assign the parallel fixtures to per-group files (avoids bash-3.2
# set -u array quirks), then launch a `test-bash-run` per group.
i=0
for name in "${PAR_FIX[@]}"; do echo "$name" >>"$OUT/grp.$(( i % JOBS ))"; i=$(( i + 1 )); done

start=$(date +%s 2>/dev/null || echo 0)
for g in $(seq 0 $((JOBS - 1))); do
  [ -f "$OUT/grp.$g" ] || continue
  grp=$(tr '\n' ' ' <"$OUT/grp.$g")
  # Per-group HOME/HISTFILE isolation: the history fixtures (histexpand, history)
  # read/write $HOME/.bash_history; round-robin places them in DIFFERENT groups,
  # so a shared HOME makes them race and flake (histexpand FAILs under -parallel
  # but PASSes serial). A private, empty HOME per group also means no ~/.inputrc
  # or ~/.bashrc bleeds in — strictly more hermetic than the ambient HOME.
  mkdir -p "$OUT/home.$g"
  ( HOME="$OUT/home.$g" HISTFILE="$OUT/home.$g/.bash_history" \
    make --no-print-directory test-bash-run \
       TESTS="$grp" BASH_TEST_SKIP="${BASH_TEST_SKIP:-}" \
       >"$OUT/g$g.out" 2>&1 ) &
done
wait

# Serial tail: the stateful history fixtures, one at a time, own private HOME.
if [ "${#SER_FIX[@]}" -gt 0 ]; then
  mkdir -p "$OUT/home.serial"
  HOME="$OUT/home.serial" HISTFILE="$OUT/home.serial/.bash_history" \
    make --no-print-directory test-bash-run \
       TESTS="${SER_FIX[*]}" BASH_TEST_SKIP="${BASH_TEST_SKIP:-}" \
       >"$OUT/gserial.out" 2>&1
fi
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
