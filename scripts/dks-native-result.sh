#!/usr/bin/env bash
# Validate one dks-native-job result from terminal Pod status.
set -euo pipefail

KUBECTL="${KUBECTL:-bashy kubectl}"
NS="${NS:-default}"
JOB="${JOB:-bashy-native}"

pod="$($KUBECTL get pods -n "$NS" -l "job-name=${JOB}" \
  -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)"
[ -n "$pod" ] || { echo "dks-native-result: no pod for job=$JOB ns=$NS" >&2; exit 2; }

phase="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.status.phase}')"
node="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.spec.nodeName}')"
message="$($KUBECTL get pod "$pod" -n "$NS" \
  -o jsonpath='{.status.containerStatuses[0].state.terminated.message}')"
record="$(printf '%s\n' "$message" | grep '^DKS_RESULT:' | tail -1 || true)"

[ "$phase" = Succeeded ] || {
  echo "dks-native-result: pod=$pod node=$node phase=$phase" >&2
  printf '%s\n' "$message" >&2
  exit 1
}
[ -n "$record" ] || {
  echo "dks-native-result: INCOMPLETE — pod=$pod node=$node has no DKS_RESULT marker" >&2
  exit 3
}
case "$record" in *'"classification":"pass"'*) ;; *)
  echo "dks-native-result: non-pass record from pod=$pod node=$node: $record" >&2
  exit 1
esac
printf 'PASS pod=%s node=%s %s\n' "$pod" "$node" "$record"
