#!/usr/bin/env bash
# Hermetic tests for examples/shaping/example-apps/run-node-ws.sh.
# Simulates a Genie tool child whose PATH places Node 22 (without ws) ahead of
# host Node 18 (with ws), plus minimal env, explicit override, and missing ws.
set -euo pipefail

here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
helper="$here/run-node-ws.sh"
[ -x "$helper" ] || chmod +x "$helper"

tmp=$(mktemp -d "${TMPDIR:-/tmp}/test-node-ws.XXXXXX")
trap 'rm -rf "$tmp"' EXIT

mkdir -p "$tmp/node22/bin" "$tmp/node18/bin" "$tmp/node18/lib/ws" "$tmp/empty18/bin"
cat >"$tmp/node18/lib/ws/index.js" <<'EOF'
module.exports = { WebSocket: function FakeWS() {}, ok: true };
EOF

# Fake Node 22.23.2 (cannot resolve ws, mimicking Bashy's managed Node 22).
cat >"$tmp/node22/bin/node" <<'EOF'
#!/usr/bin/env python3
import os, sys
if sys.argv[1] == "-e":
    sys.stderr.write("Error: Cannot find module 'ws'\n")
    sys.exit(1)
sys.stderr.write("unexpected invocation of node22\n")
sys.exit(1)
EOF

# Fake host Node 18.19.1 (resolves ws from its host module root).
cat >"$tmp/node18/bin/node" <<EOF
#!/usr/bin/env python3
import json, os, sys
ws_path = "$tmp/node18/lib/ws/index.js"
if sys.argv[1] == "-e":
    mod = sys.argv[3]
    want = os.environ.get("_E2E_WANT_MAJOR", "18")
    allow_any = os.environ.get("_E2E_ALLOW_ANY", "0") == "1"
    if not allow_any and want != "18":
        sys.stderr.write("version 18.19.1 (want major " + want + ")\n")
        sys.exit(10)
    if mod != "ws":
        sys.stderr.write("Cannot find module '" + mod + "'\n")
        sys.exit(11)
    print(json.dumps({
        "node": "$tmp/node18/bin/node",
        "version": "18.19.1",
        "module": mod,
        "resolved": ws_path,
    }))
    sys.exit(0)
script = sys.argv[1]
args = sys.argv[2:]
with open(script, "r") as fh:
    body = fh.read()
print("EXEC_NODE=18.19.1 WS=" + ws_path + " ARGS=" + " ".join(args) + " SCRIPT=" + os.path.basename(script))
EOF

# Fake Node 18 without ws installed.
cat >"$tmp/empty18/bin/node" <<'EOF'
#!/usr/bin/env python3
import sys
sys.stderr.write("Error: Cannot find module 'ws'\n")
sys.exit(1)
EOF

chmod +x "$tmp/node22/bin/node" "$tmp/node18/bin/node" "$tmp/empty18/bin/node"

cat >"$tmp/probe.js" <<'EOF'
const ws = require('ws');
console.log(Boolean(ws.ok));
EOF

# 1. PATH puts Node 22 first, host Node 18 second (candidate list avoids /usr/bin/node interference).
out=$(PATH="$tmp/node22/bin:$tmp/node18/bin:/bin:/usr/bin" \
  E2E_NODE_CANDIDATES="$tmp/node22/bin/node:$tmp/node18/bin/node" \
  bash "$helper" --preflight --json)
case "$out" in
  *'"version":'*'"18.19.1"'*'"module":'*'"ws"'*) ;;
  *) echo "FAIL test 1 preflight: $out" >&2; exit 1 ;;
esac

run_out=$(PATH="$tmp/node22/bin:$tmp/node18/bin:/bin:/usr/bin" \
  E2E_NODE_CANDIDATES="$tmp/node22/bin/node:$tmp/node18/bin/node" \
  bash "$helper" -- "$tmp/probe.js" --flag1 val1)
case "$run_out" in
  *'EXEC_NODE=18.19.1'*'ARGS=--flag1 val1'*'SCRIPT=probe.js'*) ;;
  *) echo "FAIL test 1 run: $run_out" >&2; exit 1 ;;
esac

# 2. Minimal env (simulating Genie filtered child env) still resolves Node 18 + ws.
min_out=$(env -i PATH="$tmp/node22/bin:$tmp/node18/bin:/bin:/usr/bin" \
  E2E_NODE_CANDIDATES="$tmp/node22/bin/node:$tmp/node18/bin/node" \
  bash "$helper" --print)
[ "$min_out" = "$tmp/node18/bin/node" ] || { echo "FAIL test 2 minimal env: $min_out" >&2; exit 1; }

# 3. Explicit E2E_HOST_NODE fails closed when pointed at Node 22 (no silent fallback).
set +e
bad_override=$(PATH="$tmp/node18/bin:/bin:/usr/bin" \
  E2E_HOST_NODE="$tmp/node22/bin/node" \
  bash "$helper" --preflight 2>&1)
rc=$?
set -e
[ "$rc" -eq 2 ] || { echo "FAIL test 3 expected exit 2, got $rc: $bad_override" >&2; exit 1; }
case "$bad_override" in
  *BLOCKED*E2E_HOST_NODE*) ;;
  *) echo "FAIL test 3 diagnostic: $bad_override" >&2; exit 1 ;;
esac

# 4. Node 18 without ws fails closed with exit 2 (never installs anything).
set +e
no_ws=$(PATH="/bin:/usr/bin" \
  E2E_HOST_NODE="$tmp/empty18/bin/node" \
  bash "$helper" -- "$tmp/probe.js" 2>&1)
rc=$?
set -e
[ "$rc" -eq 2 ] || { echo "FAIL test 4 expected exit 2, got $rc: $no_ws" >&2; exit 1; }
case "$no_ws" in
  *BLOCKED*"Cannot find module 'ws'"*) ;;
  *) echo "FAIL test 4 diagnostic: $no_ws" >&2; exit 1 ;;
esac

echo "test-node-ws-helper: PASS (4 checks)"
