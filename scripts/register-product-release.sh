#!/usr/bin/env bash
# Keep the existing outpost webhook contract while bashy owns the release.
set -euo pipefail
: "${REPO:?}" "${TAG:?}" "${PRERELEASE:?}" "${DIST:?}"
[[ $PRERELEASE == true || $PRERELEASE == false ]] || exit 2
# The outpost commit this tag ships: OUTPOST_COMMIT from the release job, or
# the revision bashy's go.mod pins (tool github.com/qiangli/outpost/cmd/outpost),
# expanded to the full commit.
pin=${OUTPOST_COMMIT:-}
if [[ -z $pin ]]; then
 rev=$(sed -n 's|^[[:space:]]*github.com/qiangli/outpost v[^ ]*-\([0-9a-f]\{12\}\)\( // indirect\)\{0,1\}$|\1|p' go.mod)
 auth=(); [[ -n ${GITHUB_TOKEN:-} ]] && auth=(-H "Authorization: Bearer $GITHUB_TOKEN")
 pin=$(curl -fsSL "${auth[@]}" "https://api.github.com/repos/qiangli/outpost/commits/$rev" | python3 -c 'import json,sys; print(json.load(sys.stdin).get("sha",""))')
fi
[[ $pin =~ ^[0-9a-f]{40}$ ]] || { echo 'invalid outpost source pin' >&2; exit 1; }
base=${TAG%-dev}
artifacts='{}'
for os in darwin linux windows; do
 for arch in amd64 arm64; do
  name="outpost-$base-$os-$arch"; [[ $os != windows ]] || name+=.exe
  [[ -s "$DIST/$name" ]] || { echo "missing $name" >&2; exit 1; }
  sha=$(shasum -a 256 "$DIST/$name" | awk '{print $1}')
  artifacts=$(jq -n --argjson cur "$artifacts" --arg key "${os}_${arch}" --arg url "https://github.com/$REPO/releases/download/$TAG/$name" --arg sha "$sha" '$cur + {($key): {url: $url, sha256: $sha}}')
 done
done
payload=$(mktemp)
trap 'rm -f "$payload"' EXIT
jq -n --arg tag "$TAG" --arg commit "$pin" --argjson prerelease "$PRERELEASE" --argjson artifacts "$artifacts" '{tag:$tag, commit:$commit, prerelease:$prerelease, artifacts:$artifacts}' > "$payload"
if [[ ${VALIDATE_ONLY:-0} == 1 ]]; then cat "$payload"; exit 0; fi
: "${URL:?release webhook URL required}" "${SECRET:?release webhook secret required}"
case "$URL" in
 https://*/api/webhooks/outpost-release) ;;
 *) echo 'release webhook URL must use HTTPS /api/webhooks/outpost-release; check configuration' >&2; exit 1 ;;
esac
sig=$(openssl dgst -sha256 -hmac "$SECRET" "$payload" | awk '{print $2}')
curl -fsS -X POST "$URL" -H 'Content-Type: application/json' -H "X-Outpost-Release-Sig: sha256=$sig" --data-binary "@$payload"
