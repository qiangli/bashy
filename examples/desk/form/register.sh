#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/form.bsh" --skill)
name=${1:-form}
atlas_args=()
atlas_rows=$(cd "$here/../kit" && go run ./cmd/atlas form 0)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/form.bsh" \
  --set synopsis='Ad-hoc loopback forms: define in JSON/YAML, human fills on the page, agent reads answers as JSON' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb \
  "${atlas_args[@]}" \
  --set args.positionals.1.name=id
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/form.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force

# Register through the public shaped-app path used by external apps.
"$runner" app add "$name" --type shaped --source "$here" --label "Form" --icon "📋" \
  --tip "Ad-hoc forms and questionnaires: define in JSON/YAML, human fills, agent reads answers" \
  --force

echo "Registered $name: one Bash# body, command and executable skill."
