#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bin=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/desk-notepad/notepad
mkdir -p "$(dirname "$bin")"
(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)

runner=${BASHY_BIN:-bashy}
name=${1:-notepad}
port=${NOTEPAD_PORT:-18095}

# Register command for atlas entry and CLI invocation
guide=$("$runner" --bashsharp "$here/notepad.bsh" --skill 2>/dev/null || cat << 'EOF'
View and edit any content as text (txt, md, html, json, PDF->text).
Agent drives via command/API (new, open, get, put, list, share, close); human edits Markdown in loopback page.
EOF
)

"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/notepad.bsh" \
  --set synopsis='Desk notepad: view any content as text, edit Markdown in loopback page, agent get/put/list/share' \
  --set "long=$guide" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=id --set args.flags.1.type=string \
  --set args.flags.2.name=title --set args.flags.2.type=string \
  --set args.flags.3.name=body --set args.flags.3.type=string \
  --set args.flags.4.name=file --set args.flags.4.shorthand=f --set args.flags.4.type=string \
  --set args.flags.5.name=format --set args.flags.5.type=string \
  --set args.flags.6.name=addr --set args.flags.6.type=string \
  --set args.flags.7.name=data --set args.flags.7.type=string \
  --set args.flags.8.name=prefix --set args.flags.8.type=string \
  --set args.flags.9.name=key --set args.flags.9.type=string \
  --set args.flags.10.name=break --set args.flags.10.type=string \
  --set args.positionals.0.name=verb \
  --set args.positionals.0.enum.0=serve --set args.positionals.0.enum.1=new \
  --set args.positionals.0.enum.2=open --set args.positionals.0.enum.3=get \
  --set args.positionals.0.enum.4=put --set args.positionals.0.enum.5=list \
  --set args.positionals.0.enum.6=share --set args.positionals.0.enum.7=close \
  --set args.positionals.1.name=id

# Register skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/notepad.bsh" --contract > "$tmp/skill.dhnt" 2>/dev/null || true
"$runner" skill add "$tmp" --force 2>/dev/null || true

# Register launcher tile via bashy app add
"$runner" app add "$name" --port "$port" --label "Notepad" --icon "📝" --tip "View and edit notes in Markdown, HTML, JSON, PDF" --force

echo "Registered $name -> $bin on loopback port $port"
