#!/usr/bin/env bash
# Register kb-catalog.bsh as the `bashy kb-catalog` command and skill.
# Not run by Sprint 614 S3; run it yourself when you want the short name.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/kb-catalog.bsh" --skill)
"$runner" commands add kb-catalog --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/kb-catalog.bsh" \
  --set synopsis='Render the existing KB ring as one browsable Viewer book (build/select/open) and post its link into a Klatch conversation (share)' \
  --set "long=$guide" --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
# No args schema: build/select take variadic query terms plus repeatable --tag.
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/kb-catalog.bsh" --contract > "$tmp/skill.dhnt"
"$runner" skill add "$tmp" --force
echo 'Registered kb-catalog command and skill.'
