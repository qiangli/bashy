#!/usr/bin/env bash
set -euo pipefail
export BASHY_HINTS=off
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
guide=$("$runner" --bashsharp "$here/chart.bsh" --skill)
"$runner" commands add chart --force \
 --set "exec.0=$runner" --set exec.1=--bashsharp --set "exec.2=$here/chart.bsh" \
 --set synopsis='Quick offline SVG statistics from JSON, CSV, NDJSON and OTLP metrics' \
 --set "long=$guide" --set group=platform \
 --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
 --set args.positionals.0.name=verb \
 --set args.positionals.0.enum.0=new --set args.positionals.0.enum.1=open \
 --set args.positionals.0.enum.2=get --set args.positionals.0.enum.3=put \
 --set args.positionals.0.enum.4=list --set args.positionals.0.enum.5=share \
 --set args.positionals.0.enum.6=close --set args.positionals.0.enum.7=serve \
 --set args.positionals.0.enum.8=revoke --set args.positionals.1.name=id
"$runner" app add chart --port 18098 --label Chart --icon '▥' \
 --tip 'Quick local statistics' --start "$runner" --start chart --start serve
printf '%s\n' 'Registered chart command and launcher tile. Start with bashy chart serve.'
