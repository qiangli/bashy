#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# Shared desk modules come from the local Go module proxy, never a sibling path.
. "$here/../register-modules.sh"
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/form.bsh" --skill)
name=${1:-form}
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/form.bsh" \
  --set synopsis='Ad-hoc loopback forms: define in JSON/YAML, human fills on the page, agent reads answers as JSON' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
# Stage the shapeware folder cleanly (no build/test scratch files) plus its canonical dhnt face.
for item in SKILL.md dag.md features.yaml contract.bsh shaped.json form.bsh form.sh register.sh README.md checks references examples server go.mod go.sum; do
  [[ -e "$here/$item" ]] && cp -R "$here/$item" "$tmp/$item"
done
"$runner" --bashsharp "$here/form.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force

# Register through the public shaped-app path used by external apps.
"$runner" app add "$name" --type shaped --source "$here" --label "Form" --icon "📋" \
  --tip "Ad-hoc forms and questionnaires: define in JSON/YAML, human fills, agent reads answers" \
  --force

echo "Registered $name: shapeware skill, command, and shaped app."
