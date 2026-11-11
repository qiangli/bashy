# Shared helpers for the `demo` and `go` dag targets (sourced, not executed).
: "${SESSION_DIR:=examples/session}"
: "${BASHY:=bashy}"
export BASHY_HINTS=off

session_run() { "$BASHY" --bashsharp "$SESSION_DIR/session.bsh" "$@"; }

# open_url URL: SESSION_OPEN (a script taking the URL) wins, then the platform opener.
open_url() {
  if [[ -n ${SESSION_OPEN:-} ]]; then "$SESSION_OPEN" "$1" >/dev/null 2>&1 || true; return 0; fi
  case $(uname -s) in
    Darwin) open "$1" >/dev/null 2>&1 || true ;;
    MINGW*|MSYS*|CYGWIN*) cmd.exe /c start "" "$1" >/dev/null 2>&1 || true ;;
    *) xdg-open "$1" >/dev/null 2>&1 || true ;;
  esac
}

# jget FILE EXPR: evaluate a python expression over the parsed JSON file as `d`.
jget() { python3 -c 'import json,sys; d=json.load(open(sys.argv[1])); print(eval(sys.argv[2]))' "$1" "$2"; }

# start_session GOAL PEOPLE MINUTES FORMAT AGENT PORT WORK
# plan -> apply -> next -> serve in the background; sets SPRINT, SERVE_PID and
# leaves the server links in $WORK/links.json.
start_session() {
  local goal=$1 people=$2 minutes=$3 format=$4 agent=$5 port=$6 work=$7
  local plan=(plan "$goal" --minutes "$minutes" --people "$people" --json)
  [[ -n $format ]] && plan+=(--format "$format")
  [[ -n $agent ]] && plan+=(--agent "$agent")
  session_run "${plan[@]}" > "$work/draft.json" || { echo "session: plan failed" >&2; return 1; }
  session_run apply "$work/draft.json" --json > "$work/apply.json" || { echo "session: apply failed" >&2; return 1; }
  SPRINT=$(jget "$work/apply.json" 'd["sprint"]') || return 1
  session_run next "$SPRINT" --json > "$work/next.json" || { echo "session: next failed" >&2; return 1; }
  # The server shells out to `bashy session ...`; route that to this session.bsh so
  # no registration is needed.
  local self; self=$(command -v "$BASHY") || self=$BASHY
  printf '#!/bin/sh\nif [ "$1" = session ]; then shift; exec "%s" --bashsharp "%s/session.bsh" "$@"; fi\nexec "%s" "$@"\n' \
    "$self" "$SESSION_DIR" "$self" > "$work/bashy"
  chmod +x "$work/bashy"
  local exe; exe=$(bash "$SESSION_DIR/server/run.sh" build | tail -n 1) || return 1
  BASHY_BIN=$work/bashy "$exe" serve "$SPRINT" --json --addr "0.0.0.0:${port:-18093}" \
    > "$work/links.json" 2> "$work/serve.err" &
  SERVE_PID=$!
  local i display hostport
  for i in $(seq 1 600); do
    kill -0 "$SERVE_PID" 2>/dev/null || { cat "$work/serve.err" >&2; echo "session: server exited" >&2; return 1; }
    [[ -s $work/links.json ]] && display=$(jget "$work/links.json" 'd["display"]' 2>/dev/null) && break
    sleep 0.2
  done
  [[ -n ${display:-} ]] || { echo "session: server did not start" >&2; return 1; }
  hostport=$(python3 -c 'import sys,urllib.parse as u; print(u.urlparse(sys.argv[1]).port)' "$display")
  for i in $(seq 1 100); do
    python3 -c 'import sys,urllib.request as r; r.urlopen("http://127.0.0.1:%s/healthz" % sys.argv[1], timeout=2)' "$hostport" 2>/dev/null && return 0
    sleep 0.1
  done
  echo "session: server did not become healthy" >&2
  return 1
}

# kill_tree PID: TERM then KILL PID and all its descendants (the server's own
# `bashy session ...` polling children must not outlive it).
kill_tree() {
  local pid=$1 kid
  for kid in $(pgrep -P "$pid" 2>/dev/null); do kill_tree "$kid"; done
  kill "$pid" 2>/dev/null || true
}

stop_server() {
  [[ -n ${SERVE_PID:-} ]] || return 0
  kill_tree "$SERVE_PID"
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do kill -0 "$SERVE_PID" 2>/dev/null || break; sleep 0.1; done
  kill -9 "$SERVE_PID" 2>/dev/null || true
  wait "$SERVE_PID" 2>/dev/null || true
  SERVE_PID=
}

# rm_dir DIR: remove DIR, retrying briefly while late writers drain.
rm_dir() {
  local i
  for i in 1 2 3 4 5 6 7 8 9 10; do
    rm -rf "$1" 2>/dev/null && [[ ! -e $1 ]] && return 0
    sleep 0.2
  done
  rm -rf "$1"
}

# print_links WORK SPRINT [PERSON]: labelled links, then open the OWNER page.
# Prints "<Owner role> (private): URL", "Display (projector): URL" and one
# "Phone for <name>: URL" per participant (just PERSON when given).
print_links() {
  local work=$1 sprint=$2 only=${3:-}
  session_run status "$sprint" --json > "$work/status.json" 2>/dev/null || true
  python3 - "$work/links.json" "$work/status.json" "$only" <<'PY'
import json, sys
d = json.load(open(sys.argv[1]))
try:
    role = json.load(open(sys.argv[2])).get("roles", {}).get("owner") or "host"
except Exception:
    role = "host"
print("%s (private): %s" % (role[:1].upper() + role[1:], d["owner"]))
print("Display (projector): %s" % d["display"])
for name, url in sorted(d["participants"].items()):
    if name != "owner" and (not sys.argv[3] or name == sys.argv[3]):
        print("Phone for %s: %s" % (name, url))
PY
  open_url "$(jget "$work/links.json" 'd["owner"]')"
}
