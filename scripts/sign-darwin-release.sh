#!/bin/bash
# Sign all four native Mach-O files before the release archives are made.
set -euo pipefail
[[ $# == 2 ]] || { echo "usage: $0 STAGE STATUS_JSON" >&2; exit 2; }
stage=$1 status=$2
for name in APPLE_DEVELOPER_ID_P12_BASE64 APPLE_DEVELOPER_ID_P12_PASSWORD APPLE_NOTARY_KEY_ID APPLE_NOTARY_ISSUER_ID APPLE_NOTARY_KEY_P8; do
 if [[ -z ${!name:-} ]]; then
  echo "UNSIGNED: Darwin signing credentials are incomplete"
  printf '{"signed":false}\n' > "$status"
  exit 0
 fi
done
work=$(mktemp -d "${TMPDIR:-/tmp}/bashy-sign.XXXXXX")
keychain="$work/signing.keychain-db"
cleanup() { security delete-keychain "$keychain" >/dev/null 2>&1 || true; rm -rf "$work"; }
trap cleanup EXIT
printf '%s' "$APPLE_DEVELOPER_ID_P12_BASE64" | base64 -D > "$work/developer.p12"
printf '%s' "$APPLE_NOTARY_KEY_P8" > "$work/notary.p8"
password=$(openssl rand -hex 32)
security create-keychain -p "$password" "$keychain" >/dev/null
security unlock-keychain -p "$password" "$keychain"
security import "$work/developer.p12" -k "$keychain" -P "$APPLE_DEVELOPER_ID_P12_PASSWORD" -T /usr/bin/codesign >/dev/null
security set-key-partition-list -S apple-tool:,apple: -s -k "$password" "$keychain" >/dev/null
identity=$(security find-identity -v -p codesigning "$keychain" | awk '/Developer ID Application: dhnt, Inc\./ {print $2; exit}')
[[ -n $identity ]] || { echo "Developer ID Application identity for dhnt, Inc. not found" >&2; exit 1; }
for binary in bashy outpost bash sh; do
 codesign --force --sign "$identity" --keychain "$keychain" --options runtime --timestamp "$stage/$binary"
 codesign --verify --strict --verbose=2 "$stage/$binary"
done
# Bare Mach-O files cannot be stapled. The notarization ticket remains online.
(cd "$stage" && zip -q "$work/notarize.zip" bashy outpost bash sh)
xcrun notarytool submit "$work/notarize.zip" --key "$work/notary.p8" --key-id "$APPLE_NOTARY_KEY_ID" --issuer "$APPLE_NOTARY_ISSUER_ID" --wait --output-format json > "$work/notary-result.json"
python3 - "$work/notary-result.json" <<'PY_NOTARY'
import json, sys
status = json.load(open(sys.argv[1]))['status']
if status != 'Accepted':
    raise SystemExit(f'notarization status: {status}')
PY_NOTARY
printf '{"signed":true}\n' > "$status"
