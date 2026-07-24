#!/usr/bin/env bash
# Validate a complete cargo test transcript before emitting any scoreboard.
set -euo pipefail

RAW=${1:?raw cargo log required}
FAILURES=${2:?failure output path required}
EXPECTED_RC=${3:-}
[ -f "$RAW" ] || { echo "missing cargo log: $RAW" >&2; exit 2; }

META=$(awk '
  /^test / && $0 ~ / \.\.\. (ok|FAILED|ignored)(,.*)?$/ {
    observed++
  }
  /^test result: (ok|FAILED)\./ {
    summaries++
    summary_line=NR
    status=$3
    sub(/\.$/, "", status)
    for (i=1; i<=NF; i++) {
      if ($i == "passed;") passed=$(i-1)+0
      else if ($i == "failed;") failed=$(i-1)+0
      else if ($i == "ignored;") ignored=$(i-1)+0
      else if ($i == "filtered" && $(i+1) == "out;") filtered=$(i-1)+0
    }
  }
  /^BASHY_UUTILS_CARGO_EXIT=[0-9]+$/ {
    exit_markers++
    exit_marker_line=NR
    split($0, a, "=")
    cargo=a[2]+0
  }
  /^BASHY_UUTILS_LISTED=[0-9]+$/ {
    list_markers++
    list_marker_line=NR
    split($0, a, "=")
    listed=a[2]+0
  }
  NF && exit_marker_line && NR > exit_marker_line { trailing=1 }
  END {
    if (summaries != 1 || list_markers != 1 || exit_markers != 1 ||
        summary_line >= list_marker_line || list_marker_line >= exit_marker_line || trailing)
      exit 2
    if (listed <= 0 || observed != passed+failed+ignored ||
        observed+filtered != listed)
      exit 3
    if ((status == "ok" && cargo != 0) || (status == "FAILED" && cargo != 101))
      exit 4
    printf "%d %d %d %d %d %s %d\n",
      passed, failed, ignored, observed, listed, status, cargo
  }
' "$RAW") || {
  echo "cargo transcript lacks a consistent preflight denominator/terminal summary" >&2
  exit 2
}

read -r passed failed ignored observed listed status cargo_rc <<<"$META"
if [ -n "$EXPECTED_RC" ] && [ "$cargo_rc" -ne "$EXPECTED_RC" ]; then
  echo "OCI exit $EXPECTED_RC disagrees with cargo transcript exit $cargo_rc" >&2
  exit 2
fi
awk '
  /^test test_[a-z0-9_]+::/ {
    split($2, a, "::"); mod=a[1]
    verdict=$NF
    if (verdict == "ok") pass[mod]++
    else if (verdict == "FAILED") fail[mod]++
    else if (verdict == "ignored" || $(NF-1) == "ignored,") ign[mod]++
    total[mod]++
  }
  END {
    printf "=== uutils suite scoreboard (features=unix) ===\n"
    printf "total: %d pass / %d fail / %d ignored  (%d%% of %d run)\n",
      P, F, I, (P+F ? 100*P/(P+F) : 0), P+F
    printf "--- weakest utils (fail desc) ---\n"
    for (m in total)
      if (fail[m] > 0)
        printf "%4d fail / %4d  %s\n", fail[m], pass[m]+fail[m], m | "sort -rn | head -20"
  }
' P="$passed" F="$failed" I="$ignored" "$RAW"

awk '/^test test_[a-z0-9_]+::/ && $NF == "FAILED" { print $2 }' "$RAW" | sort >"$FAILURES"
grep '^test result:' "$RAW"
