#!/bin/bash
# Decide whether Windows signing may run; absent configuration is a dev release.
set -euo pipefail
[[ $# == 1 ]] || { echo "usage: $0 STATUS_JSON" >&2; exit 2; }
for name in AZURE_CLIENT_ID AZURE_TENANT_ID AZURE_SUBSCRIPTION_ID ARTIFACT_SIGNING_ENDPOINT ARTIFACT_SIGNING_ACCOUNT ARTIFACT_SIGNING_PROFILE; do
 if [[ -z ${!name:-} ]]; then
  echo 'UNSIGNED: Windows signing credentials or configuration are incomplete'
  echo 'signed=false' >> "$GITHUB_OUTPUT"
  printf '{"signed":false}\n' > "$1"
  exit 0
 fi
done
echo 'signed=true' >> "$GITHUB_OUTPUT"
