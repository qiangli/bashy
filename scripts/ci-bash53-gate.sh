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

# make test-bash exits non-zero on any FAIL/TIME (by design). We WANT the full
# per-fixture listing regardless, so capture it and never let make's exit abort.
out=$(make test-bash BASH_TEST_SKIP="$SKIP" 2>&1) || true

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
