#!/usr/bin/env bash
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
"$runner" commands add board \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/board.bsh" \
  --set synopsis='Install and manage a personal shapeable board web app' \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=verb
echo 'Registered board custom command; run board install.'
