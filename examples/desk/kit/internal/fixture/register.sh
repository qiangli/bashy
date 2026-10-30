#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
# Register the memo fixture as an ordinary command (atlas probe for K0).
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
bin=${XDG_CACHE_HOME:-$HOME/.cache}/bashy/desk-memo/memo
(cd "$here" && GOWORK=off CGO_ENABLED=0 go build -trimpath -o "$bin" .)
runner=${BASHY_BIN:-bashy}
name=${1:-desk-memo-probe}
"$runner" commands add "$name" --force \
  --set "exec.0=$bin" \
  --set synopsis='Desk fixture: loopback memo server proving the shared kit' \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb \
  --set args.positionals.0.enum.0=serve --set args.positionals.0.enum.1=new \
  --set args.positionals.0.enum.2=open --set args.positionals.0.enum.3=get \
  --set args.positionals.0.enum.4=put --set args.positionals.0.enum.5=list \
  --set args.positionals.0.enum.6=share --set args.positionals.0.enum.7=close
echo "Registered $name -> $bin"
