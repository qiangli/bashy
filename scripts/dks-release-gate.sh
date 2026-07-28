#!/usr/bin/env bash
# Fail-closed release gate over completed DKS native and conformance Jobs.
set -euo pipefail

KUBECTL="${KUBECTL:-bashy kubectl}"
NS="${NS:-default}"
EXPECTED_SOURCE_REF="${EXPECTED_SOURCE_REF:?set EXPECTED_SOURCE_REF to the exact Bashy commit}"
NATIVE_JOBS="${NATIVE_JOBS:?set NATIVE_JOBS to space-separated vk-native Job names}"
CONFORMANCE_JOBS="${CONFORMANCE_JOBS:?set CONFORMANCE_JOBS to space-separated conformance Job names}"
REQUIRED_PLATFORMS="${REQUIRED_PLATFORMS:-linux darwin windows}"

seen=" "
native_count=0
for job in $NATIVE_JOBS; do
  native_result="$(NS="$NS" JOB="$job" KUBECTL="$KUBECTL" scripts/dks-native-result.sh)"
  printf '%s\n' "$native_result"
  # dks-native-result has already selected and validated the successful retry
  # Pod. Never re-query an unordered .items[0] and accidentally attest a failed
  # attempt from the same Job.
  record="$(printf '%s\n' "$native_result" | grep -o 'DKS_RESULT:.*' | tail -1)"
  platform="$(printf '%s\n' "$record" | sed -nE 's/.*"os":"([^"]+)".*/\1/p')"
  source_ref="$(printf '%s\n' "$record" | sed -nE 's/.*"source_ref":"([^"]*)".*/\1/p')"
  [ "$source_ref" = "$EXPECTED_SOURCE_REF" ] || {
    echo "dks-release-gate: $job tested source_ref=$source_ref, want $EXPECTED_SOURCE_REF" >&2
    exit 4
  }
  [ -n "$platform" ] || {
    echo "dks-release-gate: $job has no platform evidence" >&2
    exit 4
  }
  seen="${seen}${platform} "
  native_count=$((native_count + 1))
done

for platform in $REQUIRED_PLATFORMS; do
  case "$seen" in *" $platform "*) ;; *)
    echo "dks-release-gate: missing required vk-native platform $platform" >&2
    exit 5
  esac
done

conformance_count=0
for job in $CONFORMANCE_JOBS; do
  NS="$NS" JOB="$job" KUBECTL="$KUBECTL" scripts/k8s-job-aggregate.sh
  conformance_count=$((conformance_count + 1))
done

printf 'DKS_RELEASE_GATE:{"schema":1,"classification":"pass","source_ref":"%s","native_jobs":%d,"conformance_jobs":%d,"platforms":"%s"}\n' \
  "$EXPECTED_SOURCE_REF" "$native_count" "$conformance_count" "$REQUIRED_PLATFORMS"
