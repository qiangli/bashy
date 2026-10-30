#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/whiteboard.bsh" --skill)
"$runner" commands add whiteboard --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/whiteboard.bsh" \
  --set synopsis='Native LAN shared whiteboard: serve, status, backup, restore, stop, test; desk verbs new, open, get, put, list, share, close' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=action \
  --set args.positionals.0.enum.0=serve --set args.positionals.0.enum.1=status \
  --set args.positionals.0.enum.2=backup --set args.positionals.0.enum.3=restore \
  --set args.positionals.0.enum.4=stop --set args.positionals.0.enum.5=test \
  --set args.positionals.0.enum.6=new --set args.positionals.0.enum.7=open \
  --set args.positionals.0.enum.8=get --set args.positionals.0.enum.9=put \
  --set args.positionals.0.enum.10=list --set args.positionals.0.enum.11=share \
  --set args.positionals.0.enum.12=close \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=id --set args.flags.1.type=string \
  --set args.flags.2.name=body --set args.flags.2.type=string \
  --set args.flags.3.name=key --set args.flags.3.type=string \
  --set args.positionals.1.name=snapshot
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/whiteboard.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
port=${WHITEBOARD_PORT:-18093}
"$runner" app add whiteboard --port "$port" --label "Whiteboard" --icon "🎨" --tip "Shared whiteboard: draw and pin notes" --force
echo 'Registered whiteboard: one Bash# body, command and executable skill.'
