#!/bin/sh
# In-sandbox launcher run inside `bashy contain --net deny --dir DIR --socket SOCK`.
# Starts `bashy llm session forward`, records the container-level environment,
# exports the five external-model variables pointing at the forwarder, and runs
# `bashy genie --yolo` against the fake door.
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

fwd_out="$work_tmp/forward.out"
fwd_err="$work_tmp/forward.err"
: > "$fwd_out"
: > "$fwd_err"

"$BASHY_BIN" llm session forward --socket "$CONTAINED_GENIE_SOCKET" --listen 127.0.0.1:0 \
  > "$fwd_out" 2> "$fwd_err" &
fwd_pid=$!

cleanup() {
  if [ -n "${fwd_pid:-}" ]; then
    kill "$fwd_pid" 2>/dev/null || true
    wait "$fwd_pid" 2>/dev/null || true
  fi
}
trap cleanup EXIT INT TERM

wait_secs="${CONTAINED_GENIE_FORWARD_READY_SECONDS:-20}"
ticks=$((wait_secs * 5))
if [ "$ticks" -lt 1 ]; then
  ticks=1
fi

base_url=""
i=0
while [ "$i" -lt "$ticks" ]; do
  if ! kill -0 "$fwd_pid" 2>/dev/null; then
    echo "in_sandbox: llm session forward exited prematurely" >&2
    cat "$fwd_out" >&2 || true
    cat "$fwd_err" >&2 || true
    exit 1
  fi
  if [ -s "$fwd_out" ]; then
    first_line=$(head -n 1 "$fwd_out" | tr -d '\r')
    case "$first_line" in
      http://127.0.0.1:*/v1|http://127.0.0.1:*)
        base_url="$first_line"
        break
        ;;
    esac
  fi
  sleep 0.2
  i=$((i + 1))
done

if [ -z "$base_url" ]; then
  echo "in_sandbox: timed out after ${wait_secs}s waiting for llm session forward" >&2
  cat "$fwd_out" >&2 || true
  cat "$fwd_err" >&2 || true
  exit 1
fi

export GENIE_EXTERNAL_MODEL="fake-cligw"
export GENIE_MODEL_ID="${CONTAINED_GENIE_MODEL_ID:-fake-cligw-model}"
export GENIE_EXTERNAL_CONTEXT="${CONTAINED_GENIE_CONTEXT_TOKENS:-32768}"
export OPENAI_BASE_URL="$base_url"
export OPENAI_API_KEY="$CONTAINED_GENIE_TOKEN"
export GENIE_BAR="$CONTAINED_GENIE_GENIE_BAR"
export GENIE_TOOLCHAINS="none"
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
"$BASHY_BIN" genie --yolo "$task"
