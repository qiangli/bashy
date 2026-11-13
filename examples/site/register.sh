#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
"$runner" commands add site --force \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/site.bsh" \
  --set synopsis='Create and manage shapeable sites; chat template runs entirely on owner infrastructure' \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb --set args.positionals.1.name=name
echo 'Registered site custom command; run site new --template chat.'
