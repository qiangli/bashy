#!/usr/bin/env bash
# License gate: fail bashy/yoke/sh/outpost builds on any non-permissive license
# in Go deps or vendored code (GPL/LGPL/AGPL/MPL/CPAL/SSPL/unknown), with exact-file
# reviewed exceptions only (path + sha256 + reason, same shape as prima review).
#
# usage: scripts/verify-license-gate.sh [--report-only] [TARGETS...]
# env:   LICENSE_GATE_REPORT_ONLY=1  (report findings and exit 0 without blocking)
#        LICENSE_GATE_OUT            (directory to save findings reports)
set -euo pipefail

root=$(cd "$(dirname "$0")/.." && pwd)
cd "$root"

report_only=0
if [[ "${LICENSE_GATE_REPORT_ONLY:-0}" == "1" || "${LICENSE_GATE_REPORT_ONLY:-}" == "true" ]]; then
  report_only=1
fi

args=()
has_json=0
for arg in "$@"; do
  if [[ "$arg" == "--report-only" || "$arg" == "-report-only" ]]; then
    report_only=1
  else
    if [[ "$arg" == "--json" || "$arg" == "-json" ]]; then
      has_json=1
    fi
    args+=("$arg")
  fi
done

if (( report_only )); then
  args=("-report-only" "${args[@]}")
fi

out="${LICENSE_GATE_OUT:-}"
if [[ -n "$out" ]]; then
  mkdir -p "$out"
  # Save JSON report if not already requested as JSON
  if (( ! has_json )); then
    go run ./tools/licensegate -json "${args[@]}" > "$out/licensegate.json" 2>/dev/null || true
  fi
  # Run gate, save findings, and stream to stdout
  go run ./tools/licensegate "${args[@]}" | tee "$out/findings.txt"
else
  exec go run ./tools/licensegate "${args[@]}"
fi
