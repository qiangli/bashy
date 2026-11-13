#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
chmod +x "$here/google.bsh"

guide=$("$runner" --bashsharp "$here/google.bsh" --skill)
"$runner" commands add google --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/google.bsh" \
  --set synopsis='Google Workspace CLI wrapper for Gmail, Calendar, Tasks, Drive, and self-service BYOP onboarding' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net

# Service rods (docs/service-rod-contract.md): `bashy email` / `bashy drive`
# providers on the api route. Reads declare no mutating effect; the send rod
# declares send, so the door selects it only with --yes.
chmod +x "$here/google-rod.bsh"
for spec in google-email-read:email.read:read,exec,net google-drive-read:drive.read:read,exec,net google-email-send:email.send:read,exec,net,send; do
  IFS=: read -r name role effects <<< "$spec"
  args=() i=0
  for e in ${effects//,/ }; do args+=(--set "effects.$i=$e"); i=$((i + 1)); done
  "$runner" commands add "$name" --force \
    --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/google-rod.bsh" --set "exec.3=$role" \
    --set "synopsis=$role through Google Workspace (gws; bashy google)" --set group=platform \
    --set "provides.0=$role" --set route=api "${args[@]}" >/dev/null
done

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" skill add "$tmp" --force
echo 'Registered Google Workspace command and skill.'
