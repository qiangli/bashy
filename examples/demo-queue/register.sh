#!/usr/bin/env bash
set -euo pipefail
here=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
"$runner" --bashsharp "$here/demo-queue.bsh" build
exe=$(cd "$here" && go env GOEXE)
"$runner" commands add demo-queue --force \
  --set "exec.0=$here/.build/demo-queue$exe" \
  --set 'synopsis=Host, inspect, and run timed Outpost demos' \
  --set args.positionals.0.name=verb \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net
"$runner" app add demo-queue --type shaped --source "$here" --label "Demo Queue" --force
