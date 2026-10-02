#!/bin/bash
# The sole publish point for a -dev candidate. Prepare mutates no release;
# publish acts only after the signed manifest and all native assets verify.
set -euo pipefail
[[ $# == 1 && ( $1 == prepare || $1 == publish ) ]] || { echo "usage: $0 prepare|publish" >&2; exit 2; }
phase=$1
: "${GH_TOKEN:?}" "${REPO:?}" "${TAG:?}" "${COMMIT:?}"
[[ $TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+-dev$ ]] || { echo "invalid candidate tag" >&2; exit 2; }
state=$(gh api "repos/$REPO/releases/tags/$TAG" --jq '{draft, prerelease, target_commitish}')
[[ $(jq -r .draft <<<"$state") == true ]] || { echo "candidate release is not a draft" >&2; exit 1; }
[[ $(jq -r .prerelease <<<"$state") == true ]] || { echo "candidate release is not a prerelease" >&2; exit 1; }
ref=$(gh api "repos/$REPO/git/ref/tags/$TAG" --jq '.object')
ref_type=$(jq -r .type <<<"$ref")
ref_sha=$(jq -r .sha <<<"$ref")
if [[ $ref_type == tag ]]; then ref_sha=$(gh api "repos/$REPO/git/tags/$ref_sha" --jq '.object.sha'); fi
[[ $ref_sha == "$COMMIT" ]] || { echo "tag commit $ref_sha != workflow commit $COMMIT" >&2; exit 1; }
for arch in amd64 arm64; do
 archive="native/bashy-darwin-$arch.tar.gz"
 [[ -s $archive ]] || { echo "missing native Darwin $arch archive" >&2; exit 1; }
 tar -tzf "$archive" >/dev/null
 ./scripts/verify-release-provenance.sh "$archive"
done
if [[ $phase == prepare ]]; then
 mkdir -p release-dist
 gh release download "$TAG" -R "$REPO" -D release-dist
 [[ ! -e release-dist/bashy-darwin-amd64.tar.gz && ! -e release-dist/bashy-darwin-arm64.tar.gz ]] || { echo "native archives already attached" >&2; exit 1; }
 cp native/bashy-darwin-*.tar.gz release-dist/
 (cd release-dist && sha256sum bash-* bashy-* > checksums.txt)
 ./scripts/verify-release-asset-set.sh release-dist
 exit 0
fi
./scripts/verify-release-asset-set.sh release-dist
for arch in amd64 arm64; do cmp "native/bashy-darwin-$arch.tar.gz" "release-dist/bashy-darwin-$arch.tar.gz"; done
./scripts/verify-release-provenance.sh release-dist/checksums.txt
# Upload in a draft; a failed upload cannot expose an incomplete candidate.
gh release upload "$TAG" -R "$REPO" native/bashy-darwin-*.tar.gz --clobber
gh release upload "$TAG" -R "$REPO" release-dist/checksums.txt --clobber
mkdir -p published-dist
gh release download "$TAG" -R "$REPO" -D published-dist
./scripts/verify-release-asset-set.sh published-dist
cmp release-dist/checksums.txt published-dist/checksums.txt
for arch in amd64 arm64; do
 cmp "native/bashy-darwin-$arch.tar.gz" "published-dist/bashy-darwin-$arch.tar.gz"
 ./scripts/verify-release-provenance.sh "published-dist/bashy-darwin-$arch.tar.gz"
done
./scripts/verify-release-provenance.sh published-dist/checksums.txt
gh release view "$TAG" -R "$REPO" --json body --jq .body > release-notes.md
printf '\n%s\n' "$SEED_SUMMARY" >> release-notes.md
gh release edit "$TAG" -R "$REPO" --notes-file release-notes.md
gh release edit "$TAG" -R "$REPO" --draft=false --prerelease
