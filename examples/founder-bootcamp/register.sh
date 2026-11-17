#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/bootcamp.bsh" --skill)
"$runner" commands add founder-bootcamp --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/bootcamp.bsh" \
  --set synopsis='Founder bootcamp: company, patents, funding — progress, deadlines, browser takeover, deck and demo' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb --set args.positionals.1.name=arg1 \
  --set args.positionals.2.name=arg2 --set args.positionals.3.name=arg3 \
  --set args.flags.0.name=note --set args.flags.0.type=string \
  --set args.flags.1.name=date --set args.flags.1.type=string \
  --set args.flags.2.name=track --set args.flags.2.type=string \
  --set args.flags.3.name=deck --set args.flags.3.type=string \
  --set args.flags.4.name=voice --set args.flags.4.type=string \
  --set args.flags.5.name=guide --set args.flags.5.type=bool \
  --set args.flags.6.name=json --set args.flags.6.type=bool \
  --set args.flags.7.name=skill --set args.flags.7.type=bool \
  --set args.flags.8.name=help --set args.flags.8.type=bool
tmp=$(mktemp -d)
trap 'rm -rf "${tmp:?}"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" skill add "$tmp" --force
echo 'Registered founder-bootcamp: one Bash# body, command and skill. Set BOOTCAMP_DIR to a private directory.'
