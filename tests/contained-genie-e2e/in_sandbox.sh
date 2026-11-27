#!/bin/sh
# In-sandbox launcher run inside `bashy contain --net deny --dir DIR --socket SOCK`.
# Records the container-level environment, then hands over to the product
# launcher (`bashy genie --session-launcher`), which forwards in process and
# runs genie against the fake door.
set -eu

if [ -n "${CONTAINED_GENIE_IN_SANDBOX_BASHY:-}" ]; then
  BASHY_BIN="$CONTAINED_GENIE_IN_SANDBOX_BASHY"
elif [ -x "/.bashy/bashy" ]; then
  BASHY_BIN="/.bashy/bashy"
elif [ -n "${BASH:-}" ]; then
  case "${BASH##*/}" in
    bashy|bashy.real) BASHY_BIN="$BASH" ;;
    *) BASHY_BIN="$(command -v bashy)" ;;
  esac
else
  BASHY_BIN="$(command -v bashy)"
fi

[ -n "$BASHY_BIN" ] || { echo "in_sandbox: bashy binary not found" >&2; exit 127; }
[ -n "${CONTAINED_GENIE_SOCKET:-}" ] || { echo "in_sandbox: CONTAINED_GENIE_SOCKET is unset" >&2; exit 2; }
[ -n "${CONTAINED_GENIE_TOKEN:-}" ] || { echo "in_sandbox: CONTAINED_GENIE_TOKEN is unset" >&2; exit 2; }
[ -n "${CONTAINED_GENIE_GENIE_BAR:-}" ] || { echo "in_sandbox: CONTAINED_GENIE_GENIE_BAR is unset" >&2; exit 2; }

work_tmp="${TMPDIR:-${HOME:-/tmp}}"
mkdir -p "$work_tmp"

echo "=== CONTAIN_ENV_BEGIN ==="
env
echo "=== CONTAIN_ENV_END ==="

[ -n "${CONTAINED_GENIE_PORT:-}" ] || { echo "in_sandbox: CONTAINED_GENIE_PORT is unset" >&2; exit 2; }

# The product's own in-sandbox launcher does the forwarding in process and
# starts genie; nothing here calls an adapter-backed verb (the sandbox has no
# cache to build one in).
export OPENAI_API_KEY="$CONTAINED_GENIE_TOKEN"
export GENIE_APPROVAL="auto"
export GENIE_REQUEST_TIMEOUT_MS="${CONTAINED_GENIE_REQUEST_TIMEOUT_MS:-30000}"
export BASHY_UNATTENDED_AGENT="1"
export BASHY_HINTS="off"
export BASHY_ADVISOR="off"
export BASHY_TELEMETRY_QUIET="1"
export BASHY_HOME="$work_tmp/.bashy"
export BASHY_KB_DIR="$work_tmp/.bashy/kb"
export YCODE_DATA_DIR="$work_tmp/.local/share/ycode"

task="${CONTAINED_GENIE_TASK:-Write expected.txt in the workspace, probe the outside canary, and print the environment.}"
exec "$BASHY_BIN" genie --session-launcher "$CONTAINED_GENIE_SOCKET" "$CONTAINED_GENIE_PORT" \
  "$CONTAINED_GENIE_GENIE_BAR" "${CONTAINED_GENIE_MODEL_ID:-fake-cligw-model}" \
  "e2e-$$" "$task"
