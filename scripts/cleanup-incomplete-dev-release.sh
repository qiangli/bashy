#!/bin/bash
# On a failed workflow, remove only a draft created during THIS run. Never
# delete a published candidate or a draft from an earlier workflow attempt.
set -euo pipefail
: "${GH_TOKEN:?}" "${REPO:?}" "${TAG:?}" "${COMMIT:?}" "${RUN_ID:?}"
state=$(gh api "repos/$REPO/releases/tags/$TAG" --jq '{draft, created_at}' 2>/dev/null) || exit 0
[[ $(jq -r .draft <<<"$state") == true ]] || exit 0
created=$(jq -r .created_at <<<"$state")
started=$(gh api "repos/$REPO/actions/runs/$RUN_ID" --jq .created_at)
[[ $created > $started || $created == "$started" ]] || { echo "preserving older draft $TAG"; exit 0; }
ref=$(gh api "repos/$REPO/git/ref/tags/$TAG" --jq '.object')
sha=$(jq -r .sha <<<"$ref")
if [[ $(jq -r .type <<<"$ref") == tag ]]; then sha=$(gh api "repos/$REPO/git/tags/$sha" --jq '.object.sha'); fi
[[ $sha == "$COMMIT" ]] || { echo "preserving draft on different commit"; exit 0; }
gh release delete "$TAG" -R "$REPO" --yes
echo "removed incomplete draft $TAG from failed run $RUN_ID"
