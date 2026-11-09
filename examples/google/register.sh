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

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" skill add "$tmp" --force
echo 'Registered Google Workspace command and skill.'
