#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/matrix-chat.bsh" --skill)
"$runner" commands add matrix-chat --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/matrix-chat.bsh" \
  --set synopsis='Set up and administer independent Matrix chat' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=action --set args.positionals.1.name=config
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/matrix-chat.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
# Tool declaration and service unit shipped with this example (no longer built in).
"$runner" tool add -f "$here/tuwunel.yaml" --force
"$runner" service add -f "$here/matrix-chat.yaml" --force
echo 'Registered Matrix chat command, skill, tuwunel tool and matrix-chat service unit.'
