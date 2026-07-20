#!/usr/bin/env bash
# Aggregate a chunked DKS conformance Job's per-pod logs into one pass/fail total.
#
# Each pod printed a harness "Results: N passed, M failed, …" line for its chunk.
# The Job is a placement/retry vehicle; the authoritative verdict is the SUM over
# chunks, exactly as the serial harness reports it for the whole corpus. Reads
# logs through `outpost kubectl` (or plain kubectl via $KUBECTL).
#
# Usage:  NS=user-abc JOB=bash53-conformance k8s-job-aggregate.sh
set -euo pipefail

KUBECTL="${KUBECTL:-outpost kubectl}"
NS="${NS:-default}"
JOB="${JOB:-bash53-conformance}"

pods="$($KUBECTL get pods -n "$NS" -l "app=${JOB}" \
  --field-selector=status.phase=Succeeded \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null)"

[ -n "$pods" ] || { echo "no Succeeded pods for job=$JOB ns=$NS" >&2; exit 2; }

tp=0 tf=0 ts=0 tt=0 chunks=0 missing=0
while IFS= read -r pod; do
  [ -n "$pod" ] || continue
  line="$($KUBECTL logs "$pod" -n "$NS" 2>/dev/null | grep -E '^Results:' | tail -1 || true)"
  if [ -z "$line" ]; then
    echo "WARN: no Results line from $pod (log fetch failed?) — NOT counted" >&2
    missing=$((missing + 1))
    continue
  fi
  # "Results: 14 passed, 0 failed, 0 skipped, 0 timed out"
  p=$(printf '%s' "$line" | sed -nE 's/.* ([0-9]+) passed.*/\1/p')
  f=$(printf '%s' "$line" | sed -nE 's/.* ([0-9]+) failed.*/\1/p')
  s=$(printf '%s' "$line" | sed -nE 's/.* ([0-9]+) skipped.*/\1/p')
  t=$(printf '%s' "$line" | sed -nE 's/.* ([0-9]+) timed out.*/\1/p')
  tp=$((tp + ${p:-0})); tf=$((tf + ${f:-0})); ts=$((ts + ${s:-0})); tt=$((tt + ${t:-0}))
  chunks=$((chunks + 1))
  printf '  %-28s %s\n' "$pod" "$line"
done <<EOF
$pods
EOF

echo "----"
echo "AGGREGATE ($chunks chunks): ${tp} passed, ${tf} failed, ${ts} skipped, ${tt} timed out"
# missing logs are an evidence gap, not a pass — surface loudly and fail the gate.
if [ "$missing" -gt 0 ]; then
  echo "INCOMPLETE: ${missing} pod(s) produced no Results line — verdict is not trustworthy" >&2
  exit 3
fi
[ "$tf" -eq 0 ] && [ "$tt" -eq 0 ]
