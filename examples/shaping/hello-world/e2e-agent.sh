#!/usr/bin/env bash
# Run from a disposable, already registered and paired test installation.
set -euo pipefail
here=$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)
runner=${BASHY_BIN:-bashy}
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
# Preserve the real agent exit status. Exit zero is execution, not E2E acceptance.
"$@" < "$here/e2e-agent.md"
echo 'Agent finished; E2E acceptance requires review of every assertion and evidence artifact.' >&2
