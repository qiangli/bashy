#!/bin/bash
# On a failed workflow, remove only a draft bearing THIS run's GoReleaser
# marker. Neither time nor tag/commit alone proves ownership.
set -euo pipefail
: "${GH_TOKEN:?}" "${REPO:?}" "${TAG:?}" "${COMMIT:?}" "${RUN_ID:?}"
# releases/tags/<tag> never returns drafts; find the candidate in the list.
state=$(gh api "repos/$REPO/releases?per_page=100" --jq "map(select(.tag_name == \"$TAG\"))[0] // empty | {draft, created_at, body}" 2>/dev/null) || exit 0
[[ -n $state ]] || exit 0
[[ $(jq -r .draft <<<"$state") == true ]] || exit 0
if ! jq -r .body <<<"$state" | grep -Fxq "<!-- bashy-release-run: $RUN_ID -->"; then
 echo "preserving draft without this run's marker"
 exit 0
fi
created=$(jq -r .created_at <<<"$state")
started=$(gh api "repos/$REPO/actions/runs/$RUN_ID" --jq .created_at)
[[ $created > $started || $created == "$started" ]] || { echo "preserving older draft $TAG"; exit 0; }
ref=$(gh api "repos/$REPO/git/ref/tags/$TAG" --jq '.object')
sha=$(jq -r .sha <<<"$ref")
if [[ $(jq -r .type <<<"$ref") == tag ]]; then sha=$(gh api "repos/$REPO/git/tags/$sha" --jq '.object.sha'); fi
[[ $sha == "$COMMIT" ]] || { echo "preserving draft on different commit"; exit 0; }
gh release delete "$TAG" -R "$REPO" --yes
echo "removed incomplete draft $TAG from failed run $RUN_ID"
