#!/usr/bin/env bash
# register.sh — registers site as an executable command, skill, and shapeable app
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}

guide=$("$runner" --bashsharp "$here/site.bsh" --skill 2>/dev/null || bash "$here/site.bsh" --skill)
name=${1:-site}
sample_dir="$here/sample"

# Build sample site once so public/ exists
bash "$here/site.bsh" build "$sample_dir" >/dev/null 2>&1 || true

# 1. Register custom command
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/site.bsh" \
  --set synopsis='Company website & web presence: Hugo generator, Sveltia CMS editor at /admin, visual page builder, and shaping lifecycle' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb \
  --set args.positionals.1.name=target

# 2. Register executable skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
("$runner" --bashsharp "$here/site.bsh" --contract 2>/dev/null || bash "$here/site.bsh" --contract) > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force

# 3. Register through the public shaped-app path used by external apps
"$runner" app add "$name" --type shaped --source "$sample_dir" --label "Site" --icon "🌐" \
  --tip "Company website & presence with embedded Sveltia CMS and visual editor" \
  --force

echo "Registered $name: custom command, executable skill, and shapeable app."
