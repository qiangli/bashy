#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/brainstorm.bsh" --skill)
atlas_args=()
atlas_rows=$(cd "$here/../desk/kit" && GOWORK=off go run ./cmd/atlas brainstorm 6)
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
  --set args.flags.5.name=preset --set args.flags.5.type=string \
  --set args.flags.6.name=preset-card --set args.flags.6.type=bool \
  --set args.flags.7.name=prior-votes --set args.flags.7.type=int \
  --set args.flags.8.name=from-session --set args.flags.8.type=string \
  --set args.flags.9.name=from-round --set args.flags.9.type=int \
  --set args.flags.10.name=from-key --set args.flags.10.type=string \
  --set args.flags.11.name=round --set args.flags.11.type=int \
  --set args.flags.12.name=phase --set args.flags.12.type=string \
  --set args.flags.13.name=next-round --set args.flags.13.type=bool \
  --set args.flags.14.name=start-phase --set args.flags.14.type=string \
  --set args.flags.15.name=min-votes --set args.flags.15.type=int \
  --set args.flags.16.name=top-n --set args.flags.16.type=int \
  --set args.flags.17.name=select --set args.flags.17.type=string \
  --set args.flags.18.name=filter-column --set args.flags.18.type=int \
  --set args.flags.19.name=filter-author --set args.flags.19.type=string \
  --set args.flags.20.name=filter-query --set args.flags.20.type=string \
  --set args.flags.21.name=keep-votes --set args.flags.21.type=bool \
  --set args.flags.22.name=reopen-round --set args.flags.22.type=int \
  --set args.flags.23.name=column --set args.flags.23.type=int \
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
