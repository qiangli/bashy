#!/usr/bin/env bash
# ci-bash53-gate.sh — the no-regression ratchet for the bash-5.3 fixture suite.
#
# WHY THIS EXISTS. `make test-bash` is the mandatory pre-release gate, but until
# 2026-07-10 its recipe ended at `echo ""` and exited 0 no matter how many
# fixtures failed, and CI never ran it at all. So four POSIX-frontier commits in
# ../sh regressed the suite from 86/86 to 79/86 completely unseen. The exit code
# is fixed now; this script puts the gate in CI without the chicken-and-egg
# problem that an absolute "86/86 or fail" check would create while the suite is
# still red — it would block every merge, including the merges that FIX it.
#
# So this is a RATCHET, not an absolute gate:
#   * a fixture that fails but is not in the baseline -> NEW regression -> fail.
#   * a baseline fixture that now passes -> progress -> fail, demanding you delete
#     its line, so the baseline only shrinks and never silently drifts stale.
#   * baseline == actual -> pass.
# When the baseline reaches empty, replace this with a plain `make test-bash`.
#
# Headless job-control fixtures (coproc, jobs, trap) hang without a controlling
# TTY on the goroutine-subshell / no-kernel-job-control constraint, so they are
# SKIPPED here and are NOT guarded by this gate — a pre-existing, documented
# limitation (see CLAUDE.md, BASH_TEST_SKIP). Guarding them needs a PTY runner.
set -uo pipefail

cd "$(dirname "$0")/.."
BASELINE=test/bash53-known-failures.txt
SKIP="coproc jobs trap"

[ -f "$BASELINE" ] || { echo "gate: missing $BASELINE" >&2; exit 2; }
[ -d "${BASH_TESTS_DIR:-external/bash-5.3/tests}" ] || {
  echo "gate: fixture tree missing (external/bash-5.3 symlink); CI must fetch it first" >&2
  exit 2
}

# Run the suite to a LOG FILE in the background and reap it as soon as the
# "Results:" line lands — never wait for make's natural exit.
#
# WHY. make test-bash spawns a per-fixture watchdog + timeout `sleep` in the
# background. Those can outlive the fixture and keep a captured pipe (`$(...)` or
# `tee`) open, so a capture hangs AFTER the suite has already finished and printed
# Results — the documented "hangs on exit after writing Results" behavior. In CI
# that presented as a 30-minute timeout with ZERO fixture lines: the suite ran,
# the capture never returned. Redirecting to a file (not a pipe) and polling the
# file sidesteps it and, as a bonus, gives live diagnostics — a genuine hang now
# shows the last fixture printed before the deadline instead of nothing.
logf=$(mktemp)
trap 'rm -f "$logf"' EXIT
: > "$logf"
make test-bash BASH_TEST_SKIP="$SKIP" BASH_TEST_TIMEOUT="${CI_BASH_TEST_TIMEOUT:-30}" \
  > "$logf" 2>&1 &
mpid=$!
deadline=$(( $(date +%s) + ${CI_GATE_DEADLINE:-1200} ))   # 20 min hard cap
last=""
while :; do
  if grep -q '^Results:' "$logf"; then break; fi
  kill -0 "$mpid" 2>/dev/null || break                    # make exited on its own
  if [ "$(date +%s)" -ge "$deadline" ]; then
    echo "gate: deadline reached before Results line; last fixture seen:" >&2
    grep -E '^  (PASS|FAIL|TIME|SKIP)  ' "$logf" | tail -1 >&2
    break
  fi
  # surface progress so a hang is visible in the CI log
  cur=$(grep -cE '^  (PASS|FAIL|TIME|SKIP)  ' "$logf")
  [ "$cur" != "$last" ] && { tail -1 "$logf"; last=$cur; }
  sleep 3
done
kill "$mpid" 2>/dev/null; wait "$mpid" 2>/dev/null        # reap make; stray timers self-expire
out=$(cat "$logf")

# An INCOMPLETE run must never be scored. If the suite did not reach its
# "Results:" line (a fixture hung past its per-test timeout — this happens on
# Linux for fixtures that FAIL on macOS, e.g. a comsub/exec parse regression that
# infinite-loops), the fixtures that never ran would look "fixed" and emit a
# bogus PROGRESS report. Refuse: report the likely-hung fixture and exit
# inconclusive. The gate can only score a run that finished.
if ! grep -q '^Results:' "$logf"; then
  echo "gate: INCONCLUSIVE — the suite did not finish (no Results line)." >&2
  echo "gate: last fixture with a verdict (the hang is the NEXT one in run-* order):" >&2
  printf '%s\n' "$out" | grep -E '^  (PASS|FAIL|TIME|SKIP)  ' | tail -1 | sed 's/^/  /' >&2
  echo "gate: cannot score a partial run; fix the hang (or skip that fixture) first." >&2
  exit 2
fi

# Actual non-PASS set = every FAIL or TIME line. (SKIP is not a failure.)
actual=$(printf '%s\n' "$out" | awk '/^  (FAIL|TIME)  /{print $2}' | sort -u)
# Baseline, stripped of comments/blanks.
baseline=$(grep -vE '^\s*(#|$)' "$BASELINE" | tr -d ' ' | sort -u)

# Sanity: the suite must have actually run. A run that produced no PASS lines
# means the fixtures were absent (the false-pass hole) — never treat that as a
# clean gate.
passcount=$(printf '%s\n' "$out" | grep -c '^  PASS  ')
if [ "$passcount" -eq 0 ]; then
  echo "gate: 0 fixtures passed — the suite did not run (missing fixtures?). Refusing to pass." >&2
  printf '%s\n' "$out" | tail -20 >&2
  exit 2
fi

new=$(comm -23 <(printf '%s\n' "$actual") <(printf '%s\n' "$baseline"))
fixed=$(comm -13 <(printf '%s\n' "$actual") <(printf '%s\n' "$baseline"))

printf '%s\n' "$out" | grep -E '^Results:' | sed 's/^/gate: /'
rc=0

if [ -n "$new" ]; then
  echo "gate: NEW regression(s) — these fixtures fail and are NOT in $BASELINE:" >&2
  printf '  %s\n' $new >&2
  echo "gate: fix the regression, or (only if intended) add it to the baseline with its cause." >&2
  rc=1
fi

if [ -n "$fixed" ]; then
  echo "gate: PROGRESS — these baseline fixtures now PASS; delete their lines from $BASELINE:" >&2
  printf '  %s\n' $fixed >&2
  rc=1
fi

[ "$rc" -eq 0 ] && echo "gate: OK — actual failure set matches the baseline ($(printf '%s' "$baseline" | grep -c .) known)."
exit $rc
