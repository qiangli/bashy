#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/form.bsh" --skill)
name=${1:-form}
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/form.bsh" \
  --set synopsis='Ad-hoc loopback forms: define in JSON/YAML, human fills on the page, agent reads answers as JSON' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb \
  --set args.positionals.0.enum.0=serve --set args.positionals.0.enum.1=new \
  --set args.positionals.0.enum.2=open --set args.positionals.0.enum.3=get \
  --set args.positionals.0.enum.4=put --set args.positionals.0.enum.5=list \
  --set args.positionals.0.enum.6=share --set args.positionals.0.enum.7=close \
  --set args.positionals.0.enum.8=wait \
  --set args.positionals.1.name=id
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/form.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
echo "Registered $name: one Bash# body, command and executable skill."
