#!/usr/bin/env bash
# Run from a disposable, already registered and paired test installation.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
scenario=${E2E_SCENARIO:-hello-world}
interface=${E2E_INTERFACE:-cli}
case "$scenario" in
  hello-world) task="$here/e2e-agent.md" ;;
  self-shape) task="$here/e2e-self-agent.md" ;;
  *) echo 'E2E_SCENARIO must be hello-world or self-shape' >&2; exit 2 ;;
esac
case "$interface" in
  cli|mcp|rest) ;;
  *) echo 'E2E_INTERFACE must be cli, mcp or rest' >&2; exit 2 ;;
esac
if [ -z "${BASHY_HOME:-}" ] || [ ! -d "$BASHY_HOME" ]; then
  echo 'BLOCKED: set BASHY_HOME to the prepared disposable installation directory.' >&2
  exit 2
fi
if [ -z "${BASHY_FLEET_DIR:-}" ] || [ ! -d "$BASHY_FLEET_DIR" ]; then
  echo 'BLOCKED: set BASHY_FLEET_DIR to the prepared disposable catalog directory.' >&2
  exit 2
fi
if [ -z "${E2E_CONTEXT:-}" ] || [ ! -f "$E2E_CONTEXT" ]; then
  echo 'BLOCKED: E2E_CONTEXT must name the operator-prepared run context (see e2e-matrix.md).' >&2
  exit 2
fi
if ! "$runner" shape --help >/dev/null 2>&1; then
  echo 'BLOCKED: this Bashy build lacks the shaping CLI (Sprint 564 H2).' >&2
  exit 2
fi
if [ "$#" -eq 0 ]; then
  echo 'Usage: bash e2e-agent.sh AGENT [ARGS...]' >&2
  echo 'Example: bash e2e-agent.sh codex exec -' >&2
  echo 'The agent command must accept the task on stdin; configure browser access and a disposable story first.' >&2
  exit 2
fi
# Feed the task to the existing real-agent launcher contract. Context contains
# resource references only: never paste cookies, tokens or account credentials.
# Preserve the real agent exit status. Exit zero is execution, not acceptance.
{
  cat "$here/e2e-interface-$interface.md"
  cat "$task"
  printf '\nOperator-prepared disposable run context:\n'
  cat "$E2E_CONTEXT"
} | "$@"
echo 'Agent finished; E2E acceptance requires review of every assertion and evidence artifact.' >&2
