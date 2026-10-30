#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bin=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/desk-viewer/viewer
mkdir -p "$(dirname "$bin")"
(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)

runner=${BASHY_BIN:-bashy}
name=${1:-viewer}
port=${VIEWER_PORT:-18096}

# Register command for atlas entry and CLI invocation
guide=$("$runner" --bashsharp "$here/viewer.bsh" --skill 2>/dev/null || cat << 'EOT'
View-only reader for reports, recaps, plans and documents agents write for humans.
Agent gives Markdown or template data (add, append, show, open, list, share, close, export); human reads in the loopback page.
EOT
)

atlas_args=()
atlas_rows=$(cd "$here/../kit" && go run ./cmd/atlas viewer 0)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/viewer.bsh" \
  --set synopsis='Desk viewer: view-only books of agent reports (Markdown or template + JSON data), paged, highlighted, exported' \
  --set "long=$guide" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=title --set args.flags.1.type=string \
  --set args.flags.2.name=body --set args.flags.2.type=string \
  --set args.flags.3.name=file --set args.flags.3.shorthand=f --set args.flags.3.type=string \
  --set args.flags.4.name=template --set args.flags.4.type=string \
  --set args.flags.5.name=data --set args.flags.5.type=string \
  --set args.flags.6.name=sprint --set args.flags.6.type=int \
  --set args.flags.7.name=session --set args.flags.7.type=string \
  --set args.flags.8.name=page --set args.flags.8.type=string \
  --set args.flags.9.name=asset --set args.flags.9.type=string \
  --set args.flags.10.name=to --set args.flags.10.type=string \
  --set args.flags.11.name=output --set args.flags.11.shorthand=o --set args.flags.11.type=string \
  --set args.flags.12.name=addr --set args.flags.12.type=string \
  --set args.flags.13.name=dir --set args.flags.13.type=string \
  --set args.flags.14.name=prefix --set args.flags.14.type=string \
  --set args.flags.15.name=key --set args.flags.15.type=string \
  --set args.positionals.0.name=verb \
  "${atlas_args[@]}" \
  --set args.positionals.1.name=id

# Register skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/viewer.bsh" --contract > "$tmp/skill.dhnt" 2>/dev/null || true
"$runner" skill add "$tmp" --force 2>/dev/null || true

# Register launcher tile via bashy app add
"$runner" app add "$name" --port "$port" --label "Viewer" --icon "👁" --tip "Read agent reports, recaps and plans; page, compare, export" --force

echo "Registered $name -> $bin on loopback port $port"
