#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/tutor.bsh" --skill)
verbs=(plan apply next done extend status nudge render prep serve join stop test)
args=()
for i in "${!verbs[@]}"; do
  args+=(--set "args.positionals.0.enum.$i=${verbs[$i]}")
done
flags=(json minutes people format agent title epic for owner by message line person edition from help goal addr prefix data done open)
for i in "${!flags[@]}"; do
  kind=string
  case ${flags[$i]} in json|line|help|done|open) kind=bool ;; esac
  args+=(--set "args.flags.$i.name=${flags[$i]}" --set "args.flags.$i.type=$kind")
done
args+=(--set args.flags.10.shorthand=m)
"$runner" commands add tutor --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/tutor.bsh" \
  --set synopsis='Plan and run timeboxed group, meeting and solo sessions on sprint cards' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=action "${args[@]}" \
  --set args.positionals.1.name=subject --set args.positionals.2.name=step
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/tutor.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
"$runner" app add tutor --port 18093 --label Tutor --icon "🎓" \
  --tip 'Timeboxed group, meeting and solo sessions' --force
echo 'Registered tutor: one Bash# body, command and skill.'
