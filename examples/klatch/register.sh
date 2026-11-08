#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/klatch.bsh" --skill)
"$runner" commands add klatch --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/klatch.bsh" \
  --set synopsis='Set up, provision and use a Klatch (Databag) chat server (participant verbs run via a Node >=22 helper)' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
# No args schema: participant verbs take variadic key=value / key:=JSON / flags.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/klatch.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
echo 'Registered Klatch command and skill.'
