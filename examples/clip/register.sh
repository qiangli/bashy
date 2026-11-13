#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/clip.bsh" --skill)
name=${1:-clips}
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/clip.bsh" \
  --set synopsis='Async clip library: queue a short clip from a video, a terminal scene or the screen; render in the background; take the finished path' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" skill add "$tmp" --force

# Register through the public shaped-app path used by external apps.
"$runner" app add "$name" --type shaped --source "$here" --label "Clips" --icon "🎬" \
  --tip "Async clip library: queue a clip, it renders in the background, play it on the page" \
  --force

echo "Registered $name: one Bash# body, command, executable skill and shaped app."
