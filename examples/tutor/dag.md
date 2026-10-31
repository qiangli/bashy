---
name: tutor
description: Isolated Session rehearsal and command/skill parity
type: dag
default: dry-run
vars:
  - TUTOR_DIR ?= examples/tutor
  - GOAL ?=
  - PEOPLE ?=
  - MINUTES ?=
  - FORMAT ?=
  - AGENT ?=
  - PORT ?=
---

# Session

The rehearsal plans and demonstrates a small improvement using the default format. The verifier owns a fresh temporary home and git repo for each lane;
all state is removed on exit. No live session accounts or boards are accessed.

## Tasks

### dry-run
Run both lanes in order. Each lane executes plan → reviewed five-step agenda
→ apply → next/done → status --line and checks command/skill duality.

```bash
python3 ./verify-duality.py
```

### no-agent
Offline template plan. Continue with next/done and verify Step 2/5 while all eight steps stay in JSON.

```bash
python3 ./verify-duality.py --mode none
```

### stub-agent
Stub chat supplies a draft; no real agent is launched.

```bash
python3 ./verify-duality.py --mode stub
```

### test
Run the focused tutor suite, including isolated registration and both lanes.

```bash
python3 -m unittest discover -s tests -t . -v
```

**One-command flows.**

`bashy tutor demo` and `bashy tutor go` only run these two targets, so
`bashy dag examples/tutor/dag.md demo` is the same flow. Params are make-style
`KEY=VALUE`: GOAL, PEOPLE, MINUTES, FORMAT, AGENT, PORT.

### demo
Throwaway session for a quick look: every store lives in a temporary directory
that is removed on Ctrl-C. Nothing reaches your real stores or board.

```bash
set -euo pipefail
. "$TUTOR_DIR/lib/flow.sh"
TUTOR_DIR=$(cd "$TUTOR_DIR" && pwd)
real_cache=${XDG_CACHE_HOME:-$HOME/.cache}
# Build the server (cached in the real cache dir) before the home is swapped.
server_bin=${TUTOR_SERVER_BIN:-$(XDG_CACHE_HOME=$real_cache bash "$TUTOR_DIR/server/run.sh" build | tail -n 1)}
sandbox=$(mktemp -d)
SERVE_PID=
cleanup() { stop_server; cd /; rm -rf "$sandbox"; }
trap cleanup EXIT
trap 'exit 130' INT TERM
export HOME=$sandbox/home BASHY_HOME=$sandbox/home BASHY_COORD_DIR=$sandbox/coord \
  BASHY_ROOM_DIR=$sandbox/room BASHY_APPS_DIR=$sandbox/apps XDG_DATA_HOME=$sandbox/xdg \
  XDG_CACHE_HOME=$sandbox/cache BASHY_SPRINT_ANNOUNCE=0 TUTOR_SERVER_BIN=$server_bin
export TUTOR_AGENT=
mkdir -p "$HOME" "$sandbox/repo" "$sandbox/work"
cd "$sandbox/repo"
git init -q . 2>/dev/null || true
start_session "${GOAL:-Build and demonstrate a small improvement}" alice,bob "${MINUTES:-60}" "" "" "${PORT:-0}" "$sandbox/work"
display=$(jget "$sandbox/work/links.json" 'd["display"]')
phone=$(jget "$sandbox/work/links.json" 'd["participants"]["alice"]')
open_url "$display"
printf '%s\n%s\nCtrl-C to stop\n' "$display" "$phone"
wait "$SERVE_PID" || true
```

### go
Real session in your own home: plan, apply, start step 1, serve and open the
Display page. Session data stays after Ctrl-C.

```bash
set -euo pipefail
. "$TUTOR_DIR/lib/flow.sh"
: "${GOAL:?usage: bashy tutor go \"GOAL\" --people a,b}"
: "${PEOPLE:?usage: bashy tutor go \"GOAL\" --people a,b}"
work=$(mktemp -d)
SERVE_PID=
cleanup() { stop_server; rm -rf "$work"; }
trap cleanup EXIT
trap 'exit 130' INT TERM
start_session "$GOAL" "$PEOPLE" "${MINUTES:-60}" "${FORMAT:-}" "${AGENT:-}" "${PORT:-18093}" "$work"
display=$(jget "$work/links.json" 'd["display"]')
open_url "$display"
echo "Session: sprint $SPRINT (later: bashy tutor status $SPRINT)"
echo "Display: $display"
echo "Owner (keep private): $(jget "$work/links.json" 'd["owner"]')"
jget "$work/links.json" '"\n".join("Join %s: %s" % kv for kv in sorted(d["participants"].items()) if kv[0] != "owner")'
echo "Ctrl-C to stop serving (session data stays)"
wait "$SERVE_PID" || true
```
