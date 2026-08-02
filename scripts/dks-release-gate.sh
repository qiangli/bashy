#!/usr/bin/env bash
# Fail-closed release gate over completed DKS native and conformance Jobs.
set -euo pipefail

. "$(cd "$(dirname "$0")" && pwd)/dks-profile.sh"
dks_kubectl_init "bashy kubectl" || exit $?
# Hand the resolved argv down to the child DKS scripts argv-safe (a newline per
# element), so a whitespace/glob kubeconfig path survives the process boundary.
DKS_KUBECTL_ARGV="$(dks_kubectl_serialize)"
DHNT="${DHNT:-bashy dhnt}"
NS="${NS:-default}"
EXPECTED_SOURCE_REF="${EXPECTED_SOURCE_REF:?set EXPECTED_SOURCE_REF to the exact Bashy commit}"
EXPECTED_SOURCE_SHA256="${EXPECTED_SOURCE_SHA256:?set EXPECTED_SOURCE_SHA256 to the source-tree sha256}"
EXPECTED_SOURCE_REPOSITORY="${EXPECTED_SOURCE_REPOSITORY:-https://github.com/qiangli/bashy.git}"
PIPELINE_FILE="${PIPELINE_FILE:?set PIPELINE_FILE to the exact dhnt.pipeline/v1 plan}"
NATIVE_JOBS="${NATIVE_JOBS:?set NATIVE_JOBS to space-separated vk-native Job names}"
CONFORMANCE_JOBS="${CONFORMANCE_JOBS:?set CONFORMANCE_JOBS to space-separated conformance Job names}"
REQUIRED_PLATFORMS="${REQUIRED_PLATFORMS:-linux darwin windows}"

# Shell word-splitting swallows whitespace-only values, silently dropping the
# independent release policy even though the variable appears non-empty.
# Require at least one real platform token after stripping whitespace.
if [ -z "$(printf '%s' "$REQUIRED_PLATFORMS" | tr -d '[:space:]')" ]; then
  echo "dks-release-gate: REQUIRED_PLATFORMS is empty or contains only whitespace" >&2
  exit 5
fi

tmp="$(mktemp -d)"
trap '/bin/rm -rf "$tmp"' EXIT
native_count=0
for job in $NATIVE_JOBS; do
  native_result="$(NS="$NS" JOB="$job" DKS_KUBECTL_ARGV="$DKS_KUBECTL_ARGV" DHNT="$DHNT" scripts/dks-native-result.sh)"
  printf '%s\n' "$native_result"
  # dks-native-result has already selected and validated the successful retry
  # Pod. Never re-query an unordered .items[0] and accidentally attest a failed
  # attempt from the same Job.
  record="$(printf '%s\n' "$native_result" | grep -o 'DKS_RESULT:.*' | tail -1)"
  native_count=$((native_count + 1))
  printf '%s\n' "${record#DKS_RESULT:}" >"$tmp/run-$native_count.json"
done

platform_args=()
for platform in $REQUIRED_PLATFORMS; do
  platform_args+=(--require-os "$platform")
done
aggregate="$($DHNT aggregate \
  --pipeline "$PIPELINE_FILE" \
  --expect-source-repository "$EXPECTED_SOURCE_REPOSITORY" \
  --expect-source-commit "$EXPECTED_SOURCE_REF" \
  --expect-source-sha256 "$EXPECTED_SOURCE_SHA256" \
  "${platform_args[@]}" \
  "$tmp"/run-*.json)" || {
  echo "dks-release-gate: native evidence aggregation failed closed" >&2
  exit 5
}

conformance_count=0
for job in $CONFORMANCE_JOBS; do
  NS="$NS" JOB="$job" DKS_KUBECTL_ARGV="$DKS_KUBECTL_ARGV" scripts/k8s-job-aggregate.sh
  conformance_count=$((conformance_count + 1))
done
if [ "$conformance_count" -eq 0 ]; then
  echo "dks-release-gate: zero conformance jobs — absence of evidence" >&2
  exit 6
fi

printf 'DKS_RELEASE_GATE:%s\n' "$aggregate"
