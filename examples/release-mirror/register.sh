#!/usr/bin/env bash
# Register `bashy release-mirror` (bashy as the LAN install server).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
"$runner" commands add release-mirror \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/release-mirror.bsh" \
  --set synopsis='Serve bashy releases and local builds to the LAN over HTTP (fetch/publish/serve/index/mcp)' \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
echo 'Registered the release-mirror custom command; run: bashy release-mirror --help'
