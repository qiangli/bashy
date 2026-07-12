#!/usr/bin/env bash
# ci-bash53-gate.sh — the no-regression ratchet for the bash-5.3 fixture suite.
#
# WHY THIS EXISTS. `make test-bash` is the mandatory pre-release gate. This script
# puts it in CI as a RATCHET rather than an absolute "86/86 or fail" check, so a
# red suite cannot block the very merges that would fix it:
#
#   * a fixture that fails but is NOT in the baseline -> NEW regression -> fail.
#   * a baseline fixture that now passes -> progress -> fail, demanding you delete
#     its line, so the baseline only shrinks and never silently drifts stale.
#   * baseline == actual -> pass.
#
# When the baseline reaches empty, replace this with a plain `make test-bash`.
#
# 2026-07-12: this script used to run the suite in the BACKGROUND, poll a log file
# for the "Results:" line, and give up on a 1200s deadline. All of that was a
# workaround for the shell fixture loop, which spawned per-fixture `sleep` watchdogs
# that outlived the fixture, held the captured pipe open, and hung the capture AFTER
# the suite had already finished. In CI that burned the full 20-minute deadline on
# every run and exited INCONCLUSIVE — and `continue-on-error: true` painted it green,
# so the conformance gate reported success without running for ~10 merges (a trap
# regression, 86/86 -> 85/86, walked in during that window).
#
# The shell loop is gone. There is ONE runner (tools/bash53suite) and it always
# terminates and always prints Results, so the suite runs in the foreground and the
# entire workaround is deleted.
#
# Nothing is skipped, either. coproc/jobs/trap were skipped here only because they
# HUNG under the old loop; they complete under the Go harness (which sets Setpgid
# from the PARENT, so the testee's cooperation is irrelevant, and kills the whole
# process tree). The gate now covers all 86 fixtures.
set -uo pipefail

cd "$(dirname "$0")/.."
BASELINE=test/bash53-known-failures.txt

[ -f "$BASELINE" ] || { echo "gate: missing $BASELINE" >&2; exit 2; }
[ -d "${BASH_TESTS_DIR:-external/bash-5.3/tests}" ] || {
  echo "gate: fixture tree missing (external/bash-5.3 symlink); CI must fetch it first" >&2
  exit 2
}

out=$(make test-bash BASH_TEST_TIMEOUT="${CI_BASH_TEST_TIMEOUT:-60}" 2>&1)
printf '%s\n' "$out"

# An INCOMPLETE run must never be scored: fixtures that never ran would look
# "fixed" and emit a bogus PROGRESS report.
if ! printf '%s\n' "$out" | grep -q '^Results:'; then
  echo "gate: INCONCLUSIVE — no Results line; the suite did not finish." >&2
  exit 2
fi
# A run with no PASS lines means the fixtures were absent (the false-pass hole).
if [ "$(printf '%s\n' "$out" | grep -c '^  PASS  ')" -eq 0 ]; then
  echo "gate: 0 fixtures passed — the suite did not run (missing fixtures?). Refusing to pass." >&2
  exit 2
fi

# Actual non-PASS set = every FAIL or TIME line. (SKIP is not a failure.)
actual=$(printf '%s\n' "$out" | awk '/^  (FAIL|TIME)  /{print $2}' | sort -u)
baseline=$(grep -vE '^\s*(#|$)' "$BASELINE" | tr -d ' ' | sort -u)

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
