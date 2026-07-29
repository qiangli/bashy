#!/usr/bin/env bash
# Validate one dks-native-job result from terminal Pod status.
set -euo pipefail

KUBECTL="${KUBECTL:-bashy kubectl}"
DHNT="${DHNT:-bashy dhnt}"
NS="${NS:-default}"
JOB="${JOB:-bashy-native}"

pod="$($KUBECTL get pods -n "$NS" -l "job-name=${JOB}" \
  -o jsonpath='{range .items[?(@.status.phase=="Succeeded")]}{.metadata.name}{"\n"}{end}' \
  2>/dev/null | head -1)"
if [ -z "$pod" ]; then
  # Preserve useful diagnostics while the Job is active or after all attempts
  # failed. Completed Jobs are always read from their successful attempt above.
  pod="$($KUBECTL get pods -n "$NS" -l "job-name=${JOB}" \
    -o jsonpath='{.items[0].metadata.name}' 2>/dev/null)"
fi
[ -n "$pod" ] || { echo "dks-native-result: no pod for job=$JOB ns=$NS" >&2; exit 2; }

phase="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.status.phase}')"
node="$($KUBECTL get pod "$pod" -n "$NS" -o jsonpath='{.spec.nodeName}')"
backend="$($KUBECTL get node "$node" -o jsonpath='{.metadata.labels.outpost\.dhnt\.io/backend}')"
os="$($KUBECTL get node "$node" -o jsonpath='{.metadata.labels.kubernetes\.io/os}')"
arch="$($KUBECTL get node "$node" -o jsonpath='{.metadata.labels.kubernetes\.io/arch}')"
# An absent label reads back as the empty string, and an empty --expect-* is a
# no-op in `dhnt canonicalize-run`. Left unchecked, an unlabelled node would
# silently DISABLE the executor cross-check and the record would attest itself.
# Fail closed: no live label, no evidence.
for pair in "backend=$backend" "os=$os" "arch=$arch"; do
  [ -n "${pair#*=}" ] || {
    echo "dks-native-result: node=$node is missing the ${pair%%=*} label; cannot cross-check executor" >&2
    exit 4
  }
done
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
json="${record#DKS_RESULT:}"
canonical="$(printf '%s\n' "$json" | $DHNT canonicalize-run \
  --expect-node "$node" \
  --expect-backend "$backend" \
  --expect-os "$os" \
  --expect-arch "$arch" -)" || {
  echo "dks-native-result: malformed or mismatched dhnt.run/v1 from pod=$pod node=$node" >&2
  exit 4
}
case "$canonical" in *'"class":"pass"'*) ;; *)
  echo "dks-native-result: non-pass record from pod=$pod node=$node: $canonical" >&2
  exit 1
esac
printf 'PASS pod=%s node=%s DKS_RESULT:%s\n' "$pod" "$node" "$canonical"
