#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/brainstorm.bsh" --skill)
atlas_args=()
atlas_rows=$(cd "$here/../desk/kit" && go run ./cmd/atlas brainstorm 6)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add brainstorm --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/brainstorm.bsh" \
  --set synopsis='Native LAN team brainstorming: serve, status, backup, restore, stop, test; desk verbs add, open, show, set, list, share, close' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=action \
  --set args.positionals.0.enum.0=serve --set args.positionals.0.enum.1=status \
  --set args.positionals.0.enum.2=backup --set args.positionals.0.enum.3=restore \
  --set args.positionals.0.enum.4=stop --set args.positionals.0.enum.5=test \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=id --set args.flags.1.type=string \
  --set args.flags.2.name=body --set args.flags.2.type=string \
  --set args.flags.3.name=key --set args.flags.3.type=string \
  --set args.flags.4.name=title --set args.flags.4.type=string \
  "${atlas_args[@]}" \
  --set args.positionals.1.name=snapshot
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/brainstorm.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
port=${BRAINSTORM_PORT:-18091}
"$runner" app add brainstorm --port "$port" --label "Brainstorm" --icon "💡" --tip "Team brainstorm: write privately, reveal, vote" --force
echo 'Registered brainstorm: one Bash# body, command and executable skill.'
