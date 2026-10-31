# Shared helpers for the `demo` and `go` dag targets (sourced, not executed).
: "${TUTOR_DIR:=examples/tutor}"
: "${BASHY:=bashy}"
export BASHY_HINTS=off

tutor_run() { "$BASHY" --bashsharp "$TUTOR_DIR/tutor.bsh" "$@"; }

# open_url URL: TUTOR_OPEN (a script taking the URL) wins, then the platform opener.
open_url() {
  if [[ -n ${TUTOR_OPEN:-} ]]; then "$TUTOR_OPEN" "$1" >/dev/null 2>&1 || true; return 0; fi
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
  tutor_run "${plan[@]}" > "$work/draft.json" || { echo "tutor: plan failed" >&2; return 1; }
  tutor_run apply "$work/draft.json" --json > "$work/apply.json" || { echo "tutor: apply failed" >&2; return 1; }
  SPRINT=$(jget "$work/apply.json" 'd["sprint"]') || return 1
  tutor_run next "$SPRINT" --json > "$work/next.json" || { echo "tutor: next failed" >&2; return 1; }
  # The server shells out to `bashy tutor ...`; route that to this tutor.bsh so
  # no registration is needed.
  local self; self=$(command -v "$BASHY") || self=$BASHY
  printf '#!/bin/sh\nif [ "$1" = tutor ]; then shift; exec "%s" --bashsharp "%s/tutor.bsh" "$@"; fi\nexec "%s" "$@"\n' \
    "$self" "$TUTOR_DIR" "$self" > "$work/bashy"
  chmod +x "$work/bashy"
  local exe; exe=$(bash "$TUTOR_DIR/server/run.sh" build | tail -n 1) || return 1
  BASHY_BIN=$work/bashy "$exe" serve "$SPRINT" --json --addr "0.0.0.0:${port:-18093}" \
    > "$work/links.json" 2> "$work/serve.err" &
  SERVE_PID=$!
  local i display hostport
  for i in $(seq 1 600); do
    kill -0 "$SERVE_PID" 2>/dev/null || { cat "$work/serve.err" >&2; echo "tutor: server exited" >&2; return 1; }
    [[ -s $work/links.json ]] && display=$(jget "$work/links.json" 'd["display"]' 2>/dev/null) && break
    sleep 0.2
  done
  [[ -n ${display:-} ]] || { echo "tutor: server did not start" >&2; return 1; }
  hostport=$(python3 -c 'import sys,urllib.parse as u; print(u.urlparse(sys.argv[1]).port)' "$display")
  for i in $(seq 1 100); do
    python3 -c 'import sys,urllib.request as r; r.urlopen("http://127.0.0.1:%s/healthz" % sys.argv[1], timeout=2)' "$hostport" 2>/dev/null && return 0
    sleep 0.1
  done
  echo "tutor: server did not become healthy" >&2
  return 1
}

stop_server() {
  [[ -n ${SERVE_PID:-} ]] || return 0
  kill "$SERVE_PID" 2>/dev/null || true
  wait "$SERVE_PID" 2>/dev/null || true
  SERVE_PID=
}
