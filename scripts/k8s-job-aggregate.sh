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
    # Virtual-node providers may not expose the kubelet log route. DKS Jobs opt
    # into Outpost's bounded terminal log tail, which is persisted in Pod
    # status and survives provider restart. Read the final Results line there.
    message="$($KUBECTL get pod "$pod" -n "$NS" \
      -o jsonpath='{.status.containerStatuses[0].state.terminated.message}' 2>/dev/null || true)"
    line="$(printf '%s\n' "$message" | grep -E '^Results:' | tail -1 || true)"
  fi
  if [ -z "$line" ]; then
    echo "WARN: no Results line from $pod logs or terminal status — NOT counted" >&2
    missing=$((missing + 1))
    continue
  fi
  # "Results: N passed, M failed, O skipped, P timed out"
  # Parse exactly one complete anchored Results summary; reject extra or ambiguous text.
  parsed=$(printf '%s' "$line" | sed -nE 's/^Results: ([0-9]+) passed, ([0-9]+) failed, ([0-9]+) skipped, ([0-9]+) timed out$/\1 \2 \3 \4/p')
  if [ -z "$parsed" ]; then
    echo "WARN: truncated or malformed Results line — NOT counted" >&2
    missing=$((missing + 1))
    continue
  fi
  read p f s t <<< "$parsed"
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
# An aggregate with zero executed tests is absence of evidence, not a pass.
# Skipped cases were discovered but not executed; an all-skipped summary is
# still absence of evidence rather than a passing conformance run.
if [ "$tp" -eq 0 ] && [ "$tf" -eq 0 ] && [ "$tt" -eq 0 ]; then
  echo "ABSENT: no tests were executed — verdict is not trustworthy" >&2
  exit 3
fi
# The Bash 5.3 release contract requires zero skipped tests. Executing one
# case does not turn the other skipped cases into evidence.
if [ "$ts" -gt 0 ]; then
  echo "SKIPPED: ${ts} test(s) were skipped — verdict is not trustworthy" >&2
  exit 3
fi
# Inspect the Job to verify Indexed Job completeness. An aggregator that
# counts only Succeeded pods will miss incomplete Jobs where
# status.succeeded < spec.completions. The campaign contract requires
# fail-closed aggregation: query failure, empty/malformed response, missing
# required fields, non-Indexed completionMode, and succeeded < completions
# all reject the aggregate.
job_info=$($KUBECTL get job "$JOB" -n "$NS" -o json 2>/dev/null) || {
  echo "INDEXED_CHECK_FAIL: kubectl get job failed for job=$JOB ns=$NS" >&2
  exit 3
}
[ -n "$job_info" ] || {
  echo "INDEXED_CHECK_FAIL: kubectl get job returned empty response for job=$JOB ns=$NS" >&2
  exit 3
}

completion_mode=$(printf '%s\n' "$job_info" | sed -n 's/.*"completionMode": *"\([^"]*\)".*/\1/p') || true
[ -n "$completion_mode" ] || {
  echo "INDEXED_CHECK_FAIL: missing completionMode in Job object for job=$JOB ns=$NS" >&2
  exit 3
}
[ "$completion_mode" = "Indexed" ] || {
  echo "INDEXED_CHECK_FAIL: completionMode is \"$completion_mode\", not Indexed for job=$JOB ns=$NS" >&2
  exit 3
}

spec_completions=$(printf '%s\n' "$job_info" | sed -n 's/.*"completions": *\([0-9][0-9]*\).*/\1/p') || true
[ -n "$spec_completions" ] && [ "$spec_completions" -gt 0 ] || {
  echo "INDEXED_CHECK_FAIL: missing or zero spec.completions in Job object for job=$JOB ns=$NS" >&2
  exit 3
}

status_succeeded=$(printf '%s\n' "$job_info" | sed -n 's/.*"succeeded": *\([0-9][0-9]*\).*/\1/p') || true
[ -n "$status_succeeded" ] || {
  echo "INDEXED_CHECK_FAIL: missing status.succeeded in Job object for job=$JOB ns=$NS" >&2
  exit 3
}
if [ "$status_succeeded" -lt "$spec_completions" ]; then
  echo "INCOMPLETE: ${status_succeeded} succeeded < ${spec_completions} required completions — verdict is not trustworthy" >&2
  exit 3
fi
[ "$tf" -eq 0 ] && [ "$tt" -eq 0 ]
