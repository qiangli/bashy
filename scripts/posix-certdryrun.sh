#!/usr/bin/env bash
# posix-certdryrun.sh — aggregate POSIX conformance "cert dry-run" scoreboard.
#
# Runs every conformance harness bashy has and prints ONE scoreboard, so the
# headline claim ("0 deviations across all free suites") is a single
# reproducible command. This is the agent-drivable stand-in for the licensed
# VSC-PCTS run: it catches the long tail BEFORE the 12-month Open Group license
# clock starts. See docs/conformance-statement.md + docs/plan-posix-conformance.md.
#
# Each sub-harness follows the same contract: exit 0 iff zero deviations, and
# print a `=== … ===` summary line. This driver records exit code + that summary
# per suite and fails iff any present suite has a non-zero exit (a deviation).
#
# Suites not yet built are listed as PENDING (they do not fail the run, but the
# overall verdict notes that coverage is incomplete). As the free-suite harnesses
# land (dash / modernish / austin), drop them into scripts/ with the same
# contract and they light up here automatically.
#
# Usage:  scripts/posix-certdryrun.sh [suite ...]
#   No args: run all known suites. With args: run only the named ones.
#   OCI="ycode podman" forwarded to container-based suites.
# Requires: bin/bash + bin/bashy built; a container runtime for the differential
#   suites (docker or ycode podman, auto-detected by each sub-harness).
set -u
HERE=$(cd "$(dirname "$0")/.." && pwd)
cd "$HERE" || exit 2
S="$HERE/scripts"

# Known suites, in reporting order: <key>:<script>:<one-line scope>
# A leading '@' on the key marks an INFORMATIONAL reporting harness — one that
# prints relative pass rates rather than gating on exit 0 (yash's own suite is
# the strictest POSIX shell's suite; even bash/dash sit at ~95%, so an absolute
# 0-fail gate is the wrong measure — what matters is bashy's rate vs the
# reference shells'). Informational suites are reported, never fail the verdict.
SUITES=(
  "parity:$S/posix-parity.sh:bashy --posix vs bash 5.3 --posix (mechanically-testable behaviors)"
  "parity-pty:$S/posix-parity-pty.sh:PTY-required interactive posix-mode behaviors"
  "xcu-diff:$S/posix-diff.sh:clean-room XCU corpus, 5-oracle same-env differential"
  "oils-diff:$S/oils-diff.sh:Oils spec-test case code through the live 5-shell differential"
  "multishell:$S/multishell-diff.sh:10-shell panel (strict-POSIX + feature-rich)"
  "@yash:$S/yash-posix-suite.sh:yash -p POSIX suite — INFO: bashy rate vs reference shells"
  "@dash:$S/dash-posix-suite.sh:dash function-library load check — INFO (dash ships no suite; it is an oracle)"
  "@modernish:$S/modernish-suite.sh:modernish self-test under each shell — INFO: bashy rate vs reference shells"
  "austin:$S/austin-defects.sh:Austin Group defect/interpretation corner cases — 0-gate differential"
)

WANT=("$@")
want() { [ ${#WANT[@]} -eq 0 ] && return 0; for w in "${WANT[@]}"; do [ "$w" = "$1" ] && return 0; done; return 1; }

OUTDIR=$(mktemp -d 2>/dev/null || echo /tmp/certdry.$$)
declare -a ROWS
fail=0 pending=0 ran=0 info=0

printf '\n=== POSIX cert dry-run — %s ===\n\n' "$(cd "$HERE" && (git rev-parse --short HEAD 2>/dev/null || echo '?'))"

for entry in "${SUITES[@]}"; do
  key=${entry%%:*}; rest=${entry#*:}; script=${rest%%:*}; scope=${rest#*:}
  info_only=0; case "$key" in @*) info_only=1; key=${key#@};; esac
  want "$key" || continue
  if [ ! -f "$script" ]; then
    ROWS+=("PENDING|$key|(not built)|$scope"); pending=$((pending+1)); continue
  fi
  log="$OUTDIR/$key.log"
  echo ">>> running $key …" >&2
  bash "$script" >"$log" 2>&1; rc=$?
  ran=$((ran+1))
  if [ "$info_only" = 1 ]; then
    # Reporting harness: surface bashy's own line, not a pass/fail gate.
    summary=$(grep -iE '^[[:space:]]*bashy[[:space:]]' "$log" | tail -1)
    [ -z "$summary" ] && summary=$(tail -1 "$log")
    ROWS+=("INFO|$key|$summary|$scope"); info=$((info+1)); continue
  fi
  summary=$(grep -E '^=== .* ===$' "$log" | tail -1)
  [ -z "$summary" ] && summary=$(tail -1 "$log")
  if [ "$rc" -eq 0 ]; then
    ROWS+=("PASS|$key|$summary|$scope")
  else
    ROWS+=("FAIL|$key|${summary:-exit $rc}|$scope"); fail=$((fail+1))
  fi
done

printf '%-8s  %-12s  %s\n' "STATUS" "SUITE" "SUMMARY"
printf '%-8s  %-12s  %s\n' "------" "-----" "-------"
for row in "${ROWS[@]}"; do
  IFS='|' read -r st key sm sc <<<"$row"
  printf '%-8s  %-12s  %s\n' "$st" "$key" "$sm"
done

echo
echo "logs: $OUTDIR"
echo "ran=$ran fail=$fail info=$info pending=$pending"
if [ "$fail" -gt 0 ]; then
  echo "VERDICT: $fail 0-gate suite(s) with deviations — triage before declaring."
elif [ "$pending" -gt 0 ]; then
  echo "VERDICT: 0-gate suites clean; $pending suite(s) PENDING (coverage incomplete)."
else
  echo "VERDICT: all 0-gate suites CLEAN (0 deviations)."
  echo "  $info informational suite(s) report RELATIVE measures, not a 0-gate — review the INFO rows"
  echo "  before declaring (e.g. yash bashy-vs-bash delta in yash-conformance-gap.md, modernish"
  echo "  blocker in cert-dash-sh-findings.md). The licensed VSC-PCTS run is the remaining human step."
fi
[ "$fail" -eq 0 ]
