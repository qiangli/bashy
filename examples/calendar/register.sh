#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# Shared desk modules come from the local Go module proxy, never a sibling path.
. "$here/../desk/register-modules.sh"
bin=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/desk-calendar/calendar
mkdir -p "$(dirname "$bin")"
(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)

runner=${BASHY_BIN:-bashy}
name=${1:-calendar}

# Register command for atlas entry and CLI invocation
guide=$("$runner" --bashsharp "$here/calendar.bsh" --skill 2>/dev/null || cat << 'EOF'
Self-hosted shared calendar (Desk tool) with explicit IANA timezones, agenda page,
owner/invite/member audience, Git-dir save/load, and ICS + Google Calendar interop.
EOF
)

atlas_args=()
atlas_rows=$(cd "$here/../desk/kit" && GOWORK=off CGO_ENABLED=0 go run ./cmd/atlas calendar 0)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add "$name" --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/calendar.bsh" \
  --set synopsis='Desk calendar: self-hosted agenda, explicit IANA timezones, ICS + Google import/export, Git save/load' \
  --set "long=$guide" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.flags.0.name=json --set args.flags.0.type=bool \
  --set args.flags.1.name=id --set args.flags.1.type=string \
  --set args.flags.2.name=title --set args.flags.2.type=string \
  --set args.flags.3.name=start --set args.flags.3.type=string \
  --set args.flags.4.name=end --set args.flags.4.type=string \
  --set args.flags.5.name=tz --set args.flags.5.type=string \
  --set args.flags.6.name=location --set args.flags.6.type=string \
  --set args.flags.7.name=body --set args.flags.7.type=string \
  --set args.flags.8.name=file --set args.flags.8.shorthand=f --set args.flags.8.type=string \
  --set args.flags.9.name=ref --set args.flags.9.type=string \
  --set args.flags.10.name=audience --set args.flags.10.type=string \
  --set args.flags.11.name=status --set args.flags.11.type=string \
  --set args.flags.12.name=revoke --set args.flags.12.type=string \
  --set args.flags.13.name=from --set args.flags.13.type=string \
  --set args.flags.14.name=to --set args.flags.14.type=string \
  --set args.flags.15.name=output --set args.flags.15.shorthand=o --set args.flags.15.type=string \
  --set args.flags.16.name=dir --set args.flags.16.type=string \
  --set args.flags.17.name=calendar --set args.flags.17.type=string \
  --set args.flags.18.name=all --set args.flags.18.type=bool \
  --set args.flags.19.name=addr --set args.flags.19.type=string \
  --set args.flags.20.name=data --set args.flags.20.type=string \
  --set args.flags.21.name=prefix --set args.flags.21.type=string \
  --set args.flags.22.name=key --set args.flags.22.type=string \
  --set args.positionals.0.name=verb \
  "${atlas_args[@]}" \
  --set args.positionals.1.name=id

# Register skill
tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT
printf '%s\n' "$guide" > "$tmp/SKILL.md"
"$runner" --bashsharp "$here/calendar.bsh" --contract > "$tmp/skill.dhnt" 2>/dev/null || true
"$runner" skill add "$tmp" --force 2>/dev/null || true

# Register through the public shaped-app path used by external apps.
"$runner" app add "$name" --type shaped --source "$here" \
  --label "Calendar" --icon "📅" --tip "Shared agenda, explicit timezones, ICS and Google Calendar import/export" --force

echo "Registered $name -> $bin as a shaped app"
