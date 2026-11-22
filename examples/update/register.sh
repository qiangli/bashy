#!/usr/bin/env bash
# Register `bashy update` (build bashy from main and install it; humans only).
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
"$runner" commands add update \
  --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/update.bsh" \
  --set synopsis='Build bashy from its main branch and install it over the bashy on PATH (humans only)' \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
echo 'Registered the update custom command; run: bashy update'
