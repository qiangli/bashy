#!/usr/bin/env bash
# register.sh — registers blog as an executable command, skill, and shapeable app
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}

guide=$("$runner" --bashsharp "$here/blog.bsh" --skill 2>/dev/null || bash "$here/blog.bsh" --skill)
name=${1:-blog}
sample_dir="$here/sample"

# Build the sample once so public/ exists (the shaped base commit serves it)
bash "$here/blog.bsh" build "$sample_dir" >/dev/null 2>&1 || true

# 1. Register custom command
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/blog.bsh" \
  --set synopsis='Personal blog/wiki with rich media: Markdown posts with video and voice takes, captions, terminal scenes, diagrams, code and math; builds locally, serves from this machine' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec \
  --set args.positionals.0.name=verb \
  --set args.positionals.1.name=target

# 2. Register executable skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
("$runner" --bashsharp "$here/blog.bsh" --contract 2>/dev/null || bash "$here/blog.bsh" --contract) > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force

# 3. Register through the public shaped-app path used by external apps
"$runner" app add "$name" --type shaped --source "$sample_dir" --label "Blog" --icon "📓" \
  --tip "Personal journal with rich media, editable at /admin" \
  --force

echo "Registered $name: custom command, executable skill, and shapeable app."
