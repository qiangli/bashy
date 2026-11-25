#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
# Shared desk modules come from the local Go module proxy, never a sibling path.
. "$here/../register-modules.sh"
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/chart.bsh" --skill)
"$runner" commands add chart --force \
 --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/chart.bsh" \
 --set synopsis='Quick offline SVG statistics from JSON, CSV, NDJSON and OTLP metrics' \
 --set "long=$guide" --set group=platform \
 --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
"$runner" app add chart --type shaped --source "$here" --label Chart --icon '📊' \
 --tip 'Quick local statistics' --force
printf '%s\n' 'Registered chart command and shaped app.'
