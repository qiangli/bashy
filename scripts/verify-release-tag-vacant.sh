#!/bin/bash
# GoReleaser must never append to an existing draft or published release.
set -euo pipefail
: "${GH_TOKEN:?}" "${REPO:?}" "${TAG:?}"
[[ $TAG =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-rc\.[0-9]+)?-dev$ ]] || { echo "invalid candidate tag" >&2; exit 2; }
code=$(curl --silent --show-error --output /dev/null --write-out '%{http_code}' \
  --header 'Accept: application/vnd.github+json' \
  --header "Authorization: Bearer $GH_TOKEN" \
  "https://api.github.com/repos/$REPO/releases/tags/$TAG")
case "$code" in
 404) echo "release tag $TAG is vacant";;
 200) echo "release tag $TAG already has a draft or publication; refusing to modify it" >&2; exit 1;;
 *) echo "cannot establish release vacancy: GitHub HTTP $code" >&2; exit 1;;
esac
