#!/usr/bin/env bash
# Aggregate a chunked DKS conformance Job's per-pod logs into one pass/fail total.
#
# Each pod printed a harness "Results: N passed, M failed, …" line for its chunk.
# The Job is a placement/retry vehicle; the authoritative verdict is the SUM over
# chunks, exactly as the serial harness reports it for the whole corpus. Reads
# logs through the kubectl argv resolved by dks-profile.sh (cloudbox default
# `outpost kubectl`; overridable via $KUBECTL / a peer profile).
#
# Usage:  NS=user-abc JOB=bash53-conformance k8s-job-aggregate.sh
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/dks-profile.sh"
dks_kubectl_init "outpost kubectl" || exit $?
NS="${NS:-default}"
JOB="${JOB:-bash53-conformance}"

_job_name=$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.metadata.name}' 2>/dev/null) || {
  echo "JOB_QUERY_FAIL: kubectl get job failed for job=$JOB ns=$NS" >&2
  exit 3
}
[ -n "$_job_name" ] || {
  echo "JOB_QUERY_FAIL: kubectl get job returned empty response for job=$JOB ns=$NS" >&2
  exit 3
}

job_uid=$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.metadata.uid}' 2>/dev/null) || true
[ -n "$job_uid" ] || {
  echo "JOB_UID_FAIL: missing metadata.uid in Job object for job=$JOB ns=$NS" >&2
  exit 3
}

completion_mode=$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.spec.completionMode}' 2>/dev/null) || true
[ -n "$completion_mode" ] || {
  echo "INDEXED_CHECK_FAIL: missing completionMode in Job object for job=$JOB ns=$NS" >&2
  exit 3
}
[ "$completion_mode" = "Indexed" ] || {
  echo "INDEXED_CHECK_FAIL: completionMode is \"$completion_mode\", not Indexed for job=$JOB ns=$NS" >&2
  exit 3
}

spec_completions=$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.spec.completions}' 2>/dev/null) || true
[[ "$spec_completions" =~ ^[1-9][0-9]*$ ]] || {
  echo "INDEXED_CHECK_FAIL: spec.completions must be a positive base-10 integer (got \"$spec_completions\") for job=$JOB ns=$NS" >&2
  exit 3
}

declare -a idx_seen
for ((i=0; i<spec_completions; i++)); do idx_seen[i]=0; done

pods="$(dks_kubectl get pods -n "$NS" -l "app=${JOB}" \
  --field-selector=status.phase=Succeeded \
  -o jsonpath='{range .items[*]}{.metadata.name}{"\n"}{end}' 2>/dev/null)"

[ -n "$pods" ] || { echo "no Succeeded pods for job=$JOB ns=$NS" >&2; exit 2; }

tp=0 tf=0 ts=0 tt=0 chunks=0 missing=0
while IFS= read -r pod; do
  [ -n "$pod" ] || continue

  pod_uid=$(dks_kubectl get pod "$pod" -n "$NS" \
    -o jsonpath='{.metadata.ownerReferences[0].uid}' 2>/dev/null || true)
  if [ "$pod_uid" != "$job_uid" ]; then
    echo "WARN: $pod is not owned by Job $JOB — evidence rejected" >&2
    missing=$((missing + 1))
    continue
  fi

  pod_index=$(dks_kubectl get pod "$pod" -n "$NS" \
    -o jsonpath='{.metadata.labels.batch\.kubernetes\.io/job-completion-index}' 2>/dev/null || true)
  if [ -z "$pod_index" ] || ! [[ "$pod_index" =~ ^[0-9]+$ ]]; then
    echo "WARN: $pod has absent or malformed job-completion-index — evidence rejected" >&2
    missing=$((missing + 1))
    continue
  fi
  if [ "$pod_index" -ge "$spec_completions" ]; then
    echo "WARN: $pod completion index $pod_index >= $spec_completions — out of range" >&2
    missing=$((missing + 1))
    continue
  fi
  if [ "${idx_seen[$pod_index]}" -ne 0 ]; then
    echo "WARN: duplicate completion index $pod_index from $pod — evidence rejected" >&2
    missing=$((missing + 1))
    continue
  fi
  idx_seen[$pod_index]=1

  pod_log="$(dks_kubectl logs "$pod" -n "$NS" 2>/dev/null || true)"
  results_count="$(printf '%s\n' "$pod_log" | grep -cE '^Results:' || true)"
  line=""
  if [ "$results_count" -eq 1 ]; then
    line="$(printf '%s\n' "$pod_log" | grep -E '^Results:')"
  elif [ "$results_count" -gt 1 ]; then
    echo "WARN: ${results_count} Results lines from $pod logs — ambiguous evidence; NOT counted" >&2
    missing=$((missing + 1))
    continue
  fi
  if [ -z "$line" ]; then
    # Virtual-node providers may not expose the kubelet log route. DKS Jobs opt
    # into Outpost's bounded terminal log tail, which is persisted in Pod
    # status and survives provider restart. Read the final Results line there.
    message="$(dks_kubectl get pod "$pod" -n "$NS" \
      -o jsonpath='{.status.containerStatuses[0].state.terminated.message}' 2>/dev/null || true)"
    if [ -n "$message" ]; then
      results_count="$(printf '%s\n' "$message" | grep -cE '^Results:' || true)"
      if [ "$results_count" -eq 1 ]; then
        line="$(printf '%s\n' "$message" | grep -E '^Results:')"
      elif [ "$results_count" -gt 1 ]; then
        echo "WARN: ${results_count} Results lines in terminal message — ambiguous evidence; NOT counted" >&2
        missing=$((missing + 1))
        continue
      fi
    fi
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
for ((i=0; i<spec_completions; i++)); do
  if [ "${idx_seen[$i]}" -eq 0 ]; then
    echo "INCOMPLETE: no evidence for completion index $i — verdict is not trustworthy" >&2
    exit 3
  fi
done

status_succeeded=$(dks_kubectl get job "$JOB" -n "$NS" -o jsonpath='{.status.succeeded}' 2>/dev/null) || true
[[ "$status_succeeded" =~ ^[0-9]+$ ]] || {
  echo "INDEXED_CHECK_FAIL: status.succeeded must be a non-negative base-10 integer (got \"$status_succeeded\") for job=$JOB ns=$NS" >&2
  exit 3
}
if [ "$status_succeeded" -lt "$spec_completions" ]; then
  echo "INCOMPLETE: ${status_succeeded} succeeded < ${spec_completions} required completions — verdict is not trustworthy" >&2
  exit 3
fi
if [ "$chunks" -ne "$spec_completions" ]; then
  echo "INCOMPLETE: ${chunks} observed chunk(s) != ${spec_completions} required completions — evidence is incomplete" >&2
  exit 3
fi
[ "$tf" -eq 0 ] && [ "$tt" -eq 0 ]
