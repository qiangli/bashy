#!/usr/bin/env bash
# Trusted aggregation boundary: validate exact-source DKS evidence, verify the
# published candidate points at that source, then atomically author all OS refs.
set -euo pipefail

VERSION="${VERSION:?set VERSION to the base candidate version, for example v0.19.2}"
EXPECTED_SOURCE_REF="${EXPECTED_SOURCE_REF:?set EXPECTED_SOURCE_REF to the exact Bashy commit}"
NATIVE_JOBS="${NATIVE_JOBS:?set NATIVE_JOBS to the completed native Job names}"
CONFORMANCE_JOBS="${CONFORMANCE_JOBS:?set CONFORMANCE_JOBS to the completed conformance Job names}"
PLATFORMS="${PLATFORMS:-linux darwin windows}"
REPO="${REPO:-qiangli/bashy}"
REMOTE="${REMOTE:-origin}"
GH="${GH:-gh}"
GIT="${GIT:-git}"
GATE="${GATE:-scripts/dks-release-gate.sh}"

case "$VERSION" in
  v[0-9]*.[0-9]*.[0-9]*) ;;
  *) echo "dks-author-qa-refs: VERSION must look like v1.2.3" >&2; exit 2 ;;
esac
dev="${VERSION}-dev"

is_pre="$($GH api "repos/${REPO}/releases/tags/${dev}" --jq .prerelease)"
[ "$is_pre" = true ] || {
  echo "dks-author-qa-refs: $dev is not a published prerelease" >&2
  exit 3
}
obj="$($GH api "repos/${REPO}/git/ref/tags/${dev}" --jq '.object.type + " " + .object.sha')"
kind="${obj%% *}"
candidate="${obj#* }"
if [ "$kind" = tag ]; then
  candidate="$($GH api "repos/${REPO}/git/tags/${candidate}" --jq .object.sha)"
fi
[ "$candidate" = "$EXPECTED_SOURCE_REF" ] || {
  echo "dks-author-qa-refs: $dev source=$candidate, want $EXPECTED_SOURCE_REF" >&2
  exit 4
}

NS="${NS:-default}" KUBECTL="${KUBECTL:-bashy kubectl}" \
  EXPECTED_SOURCE_REF="$EXPECTED_SOURCE_REF" \
  NATIVE_JOBS="$NATIVE_JOBS" \
  CONFORMANCE_JOBS="$CONFORMANCE_JOBS" \
  REQUIRED_PLATFORMS="$PLATFORMS" \
  "$GATE"

refs=()
for os in $PLATFORMS; do
  refs+=("${EXPECTED_SOURCE_REF}:refs/qa/${VERSION}/${os}")
done
if [ "${DRY_RUN:-0}" = 1 ]; then
  printf 'DKS_QA_REFS_READY version=%s source_ref=%s platforms=%s\n' \
    "$VERSION" "$EXPECTED_SOURCE_REF" "$PLATFORMS"
  exit 0
fi

"$GIT" push --atomic "$REMOTE" "${refs[@]}"
printf 'DKS_QA_REFS_AUTHORED version=%s source_ref=%s platforms=%s\n' \
  "$VERSION" "$EXPECTED_SOURCE_REF" "$PLATFORMS"
