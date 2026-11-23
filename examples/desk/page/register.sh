#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
name=${1:-page}

# Register command for atlas entry and CLI invocation
guide=$("$runner" --bashsharp "$here/page.bsh" --skill 2>/dev/null || cat << 'GUIDE'
One fixed, live page for a group: Markdown plus live blocks over the desk apps.
Agent drives via command/API (add, open, show, set, list, share, close, export); the group opens the page.
GUIDE
)

atlas_args=()
atlas_rows=$(cd "$here/../kit" && GOWORK=off go run ./cmd/atlas page 0)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/page.bsh" \
  --set synopsis='Desk page: one fixed, live page for a group (Markdown + live blocks), agent add/show/set/share/export' \
  --set "long=$guide" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=id --set args.flags.1.type=string \
  --set args.flags.2.name=title --set args.flags.2.type=string \
  --set args.flags.3.name=body --set args.flags.3.type=string \
  --set args.flags.4.name=file --set args.flags.4.shorthand=f --set args.flags.4.type=string \
  --set args.flags.5.name=audience --set args.flags.5.type=string \
  --set args.flags.6.name=addr --set args.flags.6.type=string \
  --set args.flags.7.name=data --set args.flags.7.type=string \
  --set args.flags.8.name=prefix --set args.flags.8.type=string \
  --set args.flags.9.name=key --set args.flags.9.type=string \
  --set args.flags.10.name=to --set args.flags.10.type=string \
  --set args.flags.11.name=output --set args.flags.11.shorthand=o --set args.flags.11.type=string \
  --set args.flags.12.name=lan --set args.flags.12.type=bool \
  --set args.positionals.0.name=verb \
  "${atlas_args[@]}" \
  --set args.positionals.1.name=id

# Register skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/page.bsh" --contract > "$tmp/skill.dhnt" 2>/dev/null || true
"$runner" skill add "$tmp" --force 2>/dev/null || true

# Register through the public shaped-app path used by external apps.
"$runner" app add "$name" --type shaped --source "$here" \
  --label "Page" --icon "📄" --tip "One live page for a group: polls, forms, notes and embeds in one link" --force

echo "Registered $name as a shaped app"
