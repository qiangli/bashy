#!/usr/bin/env bash
# Run-scoped host Node selector and ws-module preflight for shaping browser
# fixtures (Sprint 682 C4).
#
# Genie tool children can inherit Bashy's managed Node 22 on PATH while system
# JS modules such as `ws` are installed under the host Node 18 distribution.
# This helper selects a matching host Node binary and preflights `ws` module
# resolution in the exact same invocation context before running a script.
# It never installs packages and never falls back past an explicit override.
#
# Inputs (environment):
#   E2E_HOST_NODE           Optional explicit path to the host Node binary.
#                           When set, only this binary is checked (fail closed).
#   E2E_NODE_MAJOR          Required Node major version (default: 18).
#   E2E_ALLOW_ANY_NODE_MAJOR=1  Allow any Node major version that resolves the module.
#   E2E_NODE_MODULE         Module name to preflight (default: ws).
#   E2E_NODE_PATH           Optional NODE_PATH exported to both preflight and script.
#   E2E_NODE_CANDIDATES     Optional colon-separated candidate node paths to try first.
#
# Usage:
#   run-node-ws.sh --preflight [--json]
#   run-node-ws.sh --print
#   run-node-ws.sh [--] SCRIPT [ARGS...]
set -euo pipefail

usage() {
  cat >&2 <<'EOF'
Usage: run-node-ws.sh [--preflight [--json] | --print | [--] SCRIPT [ARGS...]]
Selects a host Node matching E2E_NODE_MAJOR (default 18) that resolves
E2E_NODE_MODULE (default ws) under the same invocation context.
EOF
}

mode=run
json_out=0
while [ "$#" -gt 0 ]; do
  case "$1" in
    --preflight) mode=preflight; shift ;;
    --json) json_out=1; shift ;;
    --print) mode=print; shift ;;
    --help|-h) usage; exit 0 ;;
    --) shift; break ;;
    -*) echo "BLOCKED: unknown option '$1'" >&2; usage; exit 2 ;;
    *) break ;;
  esac
done

if [ "$mode" = run ] && [ "$#" -eq 0 ]; then
  usage
  exit 2
fi

want_major=${E2E_NODE_MAJOR:-18}
allow_any=${E2E_ALLOW_ANY_NODE_MAJOR:-0}
module_name=${E2E_NODE_MODULE:-ws}
if [ -n "${E2E_NODE_PATH:-}" ]; then
  export NODE_PATH="$E2E_NODE_PATH"
fi

case "$want_major" in
  ''|*[!0-9]*)
    echo "BLOCKED: E2E_NODE_MAJOR must be numeric (got '$want_major')" >&2
    exit 2
    ;;
esac

resolve_canon() {
  local p=$1
  if command -v realpath >/dev/null 2>&1; then
    realpath "$p" 2>/dev/null || printf '%s' "$p"
  elif command -v readlink >/dev/null 2>&1; then
    readlink -f "$p" 2>/dev/null || printf '%s' "$p"
  else
    printf '%s' "$p"
  fi
}

# Preflight script runs inside the candidate Node itself under the exact
# target environment so version and require(module) are tested identically
# to the subsequent script execution.
preflight_js='
const mod = process.argv[1];
const wantMajor = process.env._E2E_WANT_MAJOR || "18";
const allowAny = process.env._E2E_ALLOW_ANY === "1";
const ver = (process.versions && process.versions.node) || "";
const major = ver.split(".")[0] || "";
if (!allowAny && major !== wantMajor) {
  console.error("version " + ver + " (want major " + wantMajor + ")");
  process.exit(10);
}
try {
  const resolved = require.resolve(mod);
  require(mod);
  process.stdout.write(JSON.stringify({
    node: process.execPath,
    version: ver,
    module: mod,
    resolved: resolved
  }) + "\n");
} catch (err) {
  console.error("module " + mod + " lookup failed under node " + ver + ": " + (err && err.message ? err.message : String(err)));
  process.exit(11);
}
'

diagnostics=()
selected_bin=""
selected_json=""

probe_candidate() {
  local cand=$1
  if [ -z "$cand" ] || [ ! -x "$cand" ] || [ -d "$cand" ]; then
    diagnostics+=("$cand: not executable")
    return 1
  fi
  local out err_file err_msg rc
  err_file=$(mktemp "${TMPDIR:-/tmp}/node-ws-preflight.XXXXXX")
  set +e
  out=$(_E2E_WANT_MAJOR="$want_major" _E2E_ALLOW_ANY="$allow_any" \
    "$cand" -e "$preflight_js" "$module_name" 2>"$err_file")
  rc=$?
  set -e
  err_msg=$(tr '\n' ' ' <"$err_file" | sed 's/  */ /g; s/ $//')
  rm -f "$err_file"
  if [ "$rc" -eq 0 ] && [ -n "$out" ]; then
    selected_bin=$cand
    selected_json=$out
    return 0
  fi
  diagnostics+=("$cand: ${err_msg:-exit $rc}")
  return 1
}

if [ -n "${E2E_HOST_NODE:-}" ]; then
  if ! probe_candidate "$E2E_HOST_NODE"; then
    echo "BLOCKED: explicit E2E_HOST_NODE failed preflight: ${diagnostics[*]}" >&2
    exit 2
  fi
else
  candidates=()
  if [ -n "${E2E_NODE_CANDIDATES:-}" ]; then
    IFS=: read -r -a extra_cands <<<"$E2E_NODE_CANDIDATES"
    for c in "${extra_cands[@]}"; do
      [ -n "$c" ] && candidates+=("$c")
    done
  fi
  for sys_cand in /usr/bin/node /usr/local/bin/node /opt/homebrew/opt/node@18/bin/node /opt/homebrew/bin/node; do
    candidates+=("$sys_cand")
  done
  if [ -n "${PATH:-}" ]; then
    IFS=: read -r -a path_dirs <<<"$PATH"
    for d in "${path_dirs[@]}"; do
      [ -n "$d" ] && [ -x "$d/node" ] && candidates+=("$d/node")
    done
  fi

  seen="|"
  for cand in "${candidates[@]}"; do
    [ -n "$cand" ] || continue
    canon=$(resolve_canon "$cand")
    case "$seen" in
      *"|$canon|"*) continue ;;
    esac
    seen="${seen}${canon}|"
    if probe_candidate "$cand"; then
      break
    fi
  done

  if [ -z "$selected_bin" ]; then
    printf 'BLOCKED: no host Node matching major=%s can load %s (checked: %s)\n' \
      "$want_major" "$module_name" "${diagnostics[*]:-none}" >&2
    exit 2
  fi
fi

case "$mode" in
  preflight)
    if [ "$json_out" -eq 1 ]; then
      printf '%s\n' "$selected_json"
    else
      printf 'NODE_BIN=%s\n' "$selected_bin"
      printf '%s\n' "$selected_json"
    fi
    ;;
  print)
    printf '%s\n' "$selected_bin"
    ;;
  run)
    exec "$selected_bin" "$@"
    ;;
esac
