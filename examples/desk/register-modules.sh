#!/usr/bin/env bash
# Publish the shared desk modules (kit, chart) to the local Go module proxy,
# so every app that requires them builds from its own directory alone: no
# sibling replace, no vendoring. Idempotent: an already published version
# with the same content is a no-op; changed content under the same version
# is refused, so a module change ships under a new version below (and the
# consumers' go.mod/go.sum move to it).
#
# Each app's register.sh sources this file first; sourced, it also exports
# GOPROXY/GONOSUMDB so the caller's own `go build` resolves the same way the
# console's shaped build does.
set -euo pipefail

desk_kit_version=v0.1.0
desk_chart_version=v0.1.0

desk_modules_here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
desk_modules_runner=${BASHY_BIN:-bashy}
desk_modules_proxy=${BASHY_GOMOD_PROXY:-${BASHY_HOME:-$HOME/.bashy}/gomod-proxy}

BASHY_HINTS=off "$desk_modules_runner" mod proxy publish "$desk_modules_here/kit" --version "$desk_kit_version" --dir "$desk_modules_proxy"
BASHY_HINTS=off "$desk_modules_runner" mod proxy publish "$desk_modules_here/chart" --version "$desk_chart_version" --dir "$desk_modules_proxy"

export GOPROXY="file://$desk_modules_proxy,${GOPROXY:-https://proxy.golang.org,direct}"
export GONOSUMDB="${GONOSUMDB:+$GONOSUMDB,}github.com/qiangli/bashy/examples"
