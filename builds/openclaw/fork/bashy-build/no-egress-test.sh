#!/usr/bin/env bash
# no-egress-test.sh — NO-TRACKING gate for the bashy OpenClaw verified build.
#
# Starts the freshly built gateway inside a Linux network namespace (only
# loopback up: no route to the outside world) under `strace -e trace=%network`,
# with a mock OTLP collector on 127.0.0.1:31415 standing in for bashy's local
# collector. FAILS (non-zero exit) if the gateway attempts ANY connection to
# an address other than loopback (127.0.0.0/8, ::1).
#
# Second layer: a NODE_OPTIONS guard hook (scripts/no-egress-guard.mjs) wraps
# fetch/http.request/https.request in the gateway process, logs every attempt
# at the update/telemetry hosts to $BASHY_EGRESS_LOG, and aborts it. The log
# must stay empty even though strace already asserts loopback-only traffic.
#
# Gateway invocation follows the pinned upstream (openclaw/openclaw v2026.10.2):
# `node openclaw.mjs gateway` reads JSON5 config from the file named by
# OPENCLAW_CONFIG_PATH (default ~/.openclaw/openclaw.json), refuses to start
# unless gateway.mode=local, binds loopback on gateway.port (default 18789),
# and answers the /healthz liveness probe as soon as the server can answer.
#
# Usage:
#   ./bashy-build/no-egress-test.sh <gateway-dir> [run-seconds]
#
# Environment (all optional):
#   GATEWAY_ARGV        argv appended to `node openclaw.mjs` (default: gateway)
#   GATEWAY_HEALTH_URL  local URL polled until the gateway is up
#                       (default: http://127.0.0.1:18789/healthz)
#   OPENCLAW_CONFIG_PATH
#                       gateway config file (default: <conf-dir>/openclaw-bashy.json)
#   OTLP_MOCK_PORT      mock collector port (default: 31415, bashy's local collector)
#   BASHY_OPENCLAW_DIR  directory holding no-tracking.env / otel.env
#                       (default: this script's directory)
#
# Fail-closed: missing strace/unshare/python3, a gateway that never comes up,
# or a positive-control egress probe that unexpectedly succeeds all FAIL.

set -euo pipefail

GATEWAY_DIR="${1:?usage: no-egress-test.sh <gateway-dir> [run-seconds]}"
RUN_SECONDS="${2:-25}"
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CONF_DIR="${BASHY_OPENCLAW_DIR:-$SCRIPT_DIR}"
GATEWAY_ARGV="${GATEWAY_ARGV:-gateway}"
GATEWAY_HEALTH_URL="${GATEWAY_HEALTH_URL:-http://127.0.0.1:18789/healthz}"
OTLP_MOCK_PORT="${OTLP_MOCK_PORT:-31415}"
OPENCLAW_CONFIG_PATH="${OPENCLAW_CONFIG_PATH:-$CONF_DIR/openclaw-bashy.json}"

for tool in unshare strace python3 node ip curl; do
  if ! command -v "$tool" >/dev/null 2>&1; then
    echo "FAIL: required tool '$tool' not found on PATH (fail-closed)" >&2
    exit 1
  fi
done

if [ ! -f "$OPENCLAW_CONFIG_PATH" ]; then
  echo "FAIL: gateway config '$OPENCLAW_CONFIG_PATH' not found (fail-closed)" >&2
  exit 1
fi

# shellcheck disable=SC1091
set -a; . "$CONF_DIR/no-tracking.env"; . "$CONF_DIR/otel.env"; set +a

WORK="$(mktemp -d)"
trap 'rm -rf "$WORK"' EXIT
TRACE="$WORK/trace.log"
MOCK_LOG="$WORK/otlp-mock.log"
READY="$WORK/ready"
# Second-layer guard: aborts + logs guarded-host attempts in-process.
# Inherited by inner.sh through the unshare environment (same filesystem).
export BASHY_EGRESS_LOG="$WORK/egress.log"
: > "$BASHY_EGRESS_LOG"
export NODE_OPTIONS="--import $CONF_DIR/scripts/no-egress-guard.mjs ${NODE_OPTIONS:-}"

# The sandbox must genuinely block egress, or a passing test proves nothing:
# a connect to a TEST-NET-1 documentation address (RFC 5737, unroutable)
# from inside the namespace must FAIL.
if unshare -rn bash -c 'ip link set lo up && python3 -c "import socket; socket.create_connection((\"192.0.2.1\", 443), timeout=5)"' 2>/dev/null; then
  echo "FAIL: sandbox positive control unexpectedly reached the network (fail-closed)" >&2
  exit 1
fi
echo "PASS: sandbox positive control confirms no route out of the namespace"

# Inner runner: everything below shares ONE net namespace, so the gateway and
# the mock collector meet on the same (isolated) loopback.
cat >"$WORK/inner.sh" <<EOF
#!/usr/bin/env bash
set -uo pipefail
cd -- "$GATEWAY_DIR"
export OPENCLAW_CONFIG_PATH="$OPENCLAW_CONFIG_PATH"
ip link set lo up
python3 "$CONF_DIR/mock-otlp-collector.py" "$OTLP_MOCK_PORT" >"$MOCK_LOG" 2>&1 &
# shellcheck disable=SC2086
strace -f -qq -e trace=%network -o "$TRACE" node openclaw.mjs $GATEWAY_ARGV &
GW=\$!
for _ in \$(seq 1 "$RUN_SECONDS"); do
  if curl -sf --max-time 2 "$GATEWAY_HEALTH_URL" >/dev/null 2>&1; then
    touch "$READY"
    break
  fi
  sleep 1
done
# Stay up a little longer so periodic jobs (startup/maintenance update checks)
# would have fired, then stop the gateway.
sleep 5
kill "\$GW" 2>/dev/null || true
wait "\$GW" 2>/dev/null || true
EOF
chmod +x "$WORK/inner.sh"

timeout "$((RUN_SECONDS + 60))" unshare -rn "$WORK/inner.sh"

if [ ! -f "$READY" ]; then
  echo "FAIL: gateway under test never answered $GATEWAY_HEALTH_URL" >&2
  echo "--- mock collector log ---"; cat "$MOCK_LOG" >&2 || true
  exit 1
fi
echo "PASS: gateway answered its local health endpoint"

# The gate: any connect() to a non-loopback address FAILS the build.
# Loopback forms: 127.x.x.x and ::1 (IPv4-mapped ::ffff:127.x included).
if grep -E 'connect\(' "$TRACE" | grep -Ev '127\.[0-9]+\.[0-9]+\.[0-9]+|::1|ffff:127' | grep -q .; then
  echo "FAIL: non-localhost connection attempt detected:" >&2
  grep -E 'connect\(' "$TRACE" | grep -Ev '127\.[0-9]+\.[0-9]+\.[0-9]+|::1|ffff:127' >&2
  exit 1
fi
echo "PASS: no connection attempts outside localhost in $RUN_SECONDS s window"
# Second-layer verdict: the in-process guard must also have seen nothing.
if [ -s "$BASHY_EGRESS_LOG" ]; then
  echo "FAIL: in-process guard logged guarded-host attempts:" >&2
  cat "$BASHY_EGRESS_LOG" >&2
  exit 1
fi
echo "PASS: in-process guard saw zero guarded-host attempts"
echo "--- OTLP mock saw (local collector path only) ---"
cat "$MOCK_LOG"
