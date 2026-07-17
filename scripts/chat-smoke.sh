#!/usr/bin/env bash
# chat-interactive smoke scoreboard — one PASS/FAIL/SKIP line.
#
# Drives `bashy chat` under a real pty against an installed agent and asserts the
# governed-launcher contract (native launch · registry · steer · capture tee ·
# teardown). INFO, never a CI gate: it needs an installed third-party agent and a
# real pty, neither of which a headless CI box has — it SKIPs cleanly there.
#
# Usage: scripts/chat-smoke.sh [agent]      (or BASHY=./bin/bashy scripts/chat-smoke.sh)
set -u

here="$(cd "$(dirname "$0")" && pwd)"
driver="$here/../tools/chat-smoke/chat_interactive_smoke.py"

if ! command -v python3 >/dev/null 2>&1; then
	echo "chat-smoke: SKIP (no python3)"
	exit 0
fi

# Prefer a repo-local build if bashy is not already on PATH.
if [ -z "${BASHY:-}" ] && ! command -v bashy >/dev/null 2>&1 && [ -x "$here/../bin/bashy" ]; then
	export BASHY="$here/../bin/bashy"
fi

python3 "$driver" "$@"
rc=$?
case "$rc" in
	0)  echo "chat-smoke: PASS" ;;
	77) echo "chat-smoke: SKIP" ; rc=0 ;;
	*)  echo "chat-smoke: FAIL" ; rc=1 ;;
esac
exit "$rc"
