#!/bin/bash
set -euo pipefail
root=$(cd "$(dirname "$0")/.." && pwd)
work=$(mktemp -d "${TMPDIR:-/tmp}/bashy-signing-test.XXXXXX")
trap 'rm -rf "$work"' EXIT
printf '{"signed":false}\n' > "$work/darwin-amd64.json"
printf '{"signed":false}\n' > "$work/darwin-arm64.json"
printf '{"signed":false}\n' > "$work/windows.json"
[[ $(python3 "$root/scripts/release-signing-status.py" "$work") == 'Signing: darwin.signed=false windows.signed=false' ]]
printf '{"signed":true}\n' > "$work/darwin-arm64.json"
if python3 "$root/scripts/release-signing-status.py" "$work" >/dev/null 2>&1; then echo 'mixed Darwin status accepted' >&2; exit 1; fi
printf 'Signing: darwin.signed=false windows.signed=false\n' > "$work/notes"
[[ $(python3 "$root/scripts/release-signing-status.py" read-note "$work/notes" windows) == false ]]
if python3 "$root/scripts/release-signing-status.py" read-note /dev/null darwin >/dev/null 2>&1; then echo 'missing release note accepted' >&2; exit 1; fi
# A credential-free candidate emits one notice per platform and records false.
env -u APPLE_DEVELOPER_ID_P12_BASE64 -u APPLE_DEVELOPER_ID_P12_PASSWORD \
    -u APPLE_NOTARY_KEY_ID -u APPLE_NOTARY_ISSUER_ID -u APPLE_NOTARY_KEY_P8 \
    "$root/scripts/sign-darwin-release.sh" "$work" "$work/darwin.json" > "$work/darwin.log"
[[ $(grep -c '^UNSIGNED:' "$work/darwin.log") == 1 ]]
[[ $(cat "$work/darwin.json") == '{"signed":false}' ]]
env -u AZURE_CLIENT_ID -u AZURE_TENANT_ID -u AZURE_SUBSCRIPTION_ID \
    -u ARTIFACT_SIGNING_ENDPOINT -u ARTIFACT_SIGNING_ACCOUNT -u ARTIFACT_SIGNING_PROFILE \
    GITHUB_OUTPUT="$work/output" "$root/scripts/check-windows-signing-config.sh" "$work/windows.json" > "$work/windows.log"
[[ $(grep -c '^UNSIGNED:' "$work/windows.log") == 1 ]]
[[ $(cat "$work/windows.json") == '{"signed":false}' ]]
[[ $(cat "$work/output") == 'signed=false' ]]
echo 'release signing manifest and credential-free path: PASS'
