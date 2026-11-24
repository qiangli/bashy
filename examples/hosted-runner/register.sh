#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
chmod +x "$here/hosted-runner.bsh"

guide=$("$runner" --bashsharp "$here/hosted-runner.bsh" --skill)
name=${1:-hosted-runner}
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/hosted-runner.bsh" \
  --set synopsis='Build and manage the containerized bashy mcp serve --hosted pilot runner' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" skill add "$tmp" --force

echo "Registered $name command and skill."
