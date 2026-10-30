#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/chart.bsh" --skill)
atlas_args=()
atlas_rows=$(cd "$here/../kit" && go run ./cmd/atlas chart 0)
while IFS= read -r row; do atlas_args+=(--set "$row"); done <<< "$atlas_rows"
"$runner" commands add chart --force \
 --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/chart.bsh" \
 --set synopsis='Quick offline SVG statistics from JSON, CSV, NDJSON and OTLP metrics' \
 --set "long=$guide" --set group=platform \
 --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
 --set args.positionals.0.name=verb \
 "${atlas_args[@]}" --set args.positionals.1.name=id
"$runner" app add chart --port 18098 --label Chart --icon '📊' \
 --tip 'Quick local statistics' --start "$runner" --start chart --start serve --force
printf '%s\n' 'Registered chart command and launcher tile. Start with bashy chart serve.'
