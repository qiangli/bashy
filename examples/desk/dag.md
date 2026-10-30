---
name: desk-roundtrip
description: Build and round-trip all six desk tools on one host, then record portable evidence
type: dag
default: roundtrip
---

# Desk acceptance recipe

Run at the repository root with `bashy dag examples/desk/dag.md roundtrip`, or
from this directory with `bashy dag roundtrip`. The manager runs the same recipe
on each claimed OS host. `EVIDENCE` chooses the one JSON output path; the default
is `./desk-roundtrip.json` in the caller's directory. Temporary data and servers
are isolated per run. The recipe uses bashy's portable userland and each tool's
own Bash# entry point; no remote transport is involved.

## Tasks

### build
Build each native CLI/server through its own Bash# or `.sh` entry point.
Effects: read, write, exec, cred, net

```bash
set -euo pipefail
export BASHY=${BASHY:-bashy}
root=$(git rev-parse --show-toplevel)
# Each entry point builds its native CLI/server cache on first dispatch. A
# deliberately unsupported JSON verb exercises that path without a server.
for tool in notepad form chart viewer whiteboard brainstorm; do
  echo "BUILD $tool"
  case "$tool" in
    notepad|form|chart|viewer) entry=$root/examples/desk/$tool/$tool.bsh ;;
    *) entry=$root/examples/$tool/$tool.bsh ;;
  esac
  if "$BASHY" --bashsharp "$entry" unsupported-desk-verb --json >/dev/null 2>&1; then
    echo "$tool: unsupported verb unexpectedly succeeded" >&2
    exit 1
  else
    [[ $? == 2 ]] || exit 1
  fi
done
```

### roundtrip
One live CLI and HTTP round-trip per tool. The final evidence is written even if
a tool fails; all six summary lines are printed, and any failure exits nonzero.
Requires: build
Effects: read, write, exec, net, cred, destroy

```bash
set -u
export BASHY=${BASHY:-bashy}
root=$(git rev-parse --show-toplevel) || exit 1
work=$("$BASHY" mktemp -d) || exit 1
evidence=${EVIDENCE:-./desk-roundtrip.json}
os=$("$BASHY" inspect context --json | "$BASHY" jq -r '.runtime.goos')
arch=$("$BASHY" inspect context --json | "$BASHY" jq -r '.runtime.goarch')
version=$("$BASHY" --version | "$BASHY" head -n 1)
results=$work/results.jsonl
: > "$results"
failed=0

check_json() {
  local tool=$1 json=$2
  printf '%s\n' "$json" | "$BASHY" jq -e ".schema_version == \"bashy-$tool-v1\"" >/dev/null
}
field() { printf '%s\n' "$1" | "$BASHY" jq -r ".$2"; }
cmd() {
  local tool=$1 result status
  shift
  case "$tool" in
    notepad) result=$("$BASHY" --bashsharp "$root/examples/desk/notepad/notepad.bsh" "$@") && status=0 || status=$? ;;
    form) result=$("$BASHY" --bashsharp "$root/examples/desk/form/form.bsh" "$@") && status=0 || status=$? ;;
    chart) result=$("$BASHY" --bashsharp "$root/examples/desk/chart/chart.bsh" --addr "$addr" --data "$data" "$@") && status=0 || status=$? ;;
    viewer) result=$("$BASHY" --bashsharp "$root/examples/desk/viewer/viewer.bsh" "$@") && status=0 || status=$? ;;
    whiteboard) result=$("$BASHY" --bashsharp "$root/examples/whiteboard/whiteboard.bsh" "$@") && status=0 || status=$? ;;
    brainstorm) result=$("$BASHY" --bashsharp "$root/examples/brainstorm/brainstorm.bsh" "$@") && status=0 || status=$? ;;
  esac
  check_json "$tool" "$result" || { echo "$tool: wrong JSON schema: $result" >&2; return 1; }
  printf '%s\n' "$result"
  return "$status"
}
serve_tool() {
  local cache=${XDG_CACHE_HOME:-$HOME/.cache}/bashy
  case "$1" in
    notepad) exec "$cache/desk-notepad/notepad" serve --addr "$addr" --data "$data" ;;
    form) exec "$cache/form/form" serve -addr "$addr" -data "$data" ;;
    chart) exec "$cache/desk-chart/chart" --addr "$addr" --data "$data" serve ;;
    viewer) exec "$cache/desk-viewer/viewer" serve --addr "$addr" --dir "$data" ;;
    whiteboard) exec "$cache/whiteboard/whiteboard" -data "$data" -addr "$addr" ;;
    brainstorm) exec "$cache/brainstorm/brainstorm" -data "$data" -addr "$addr" ;;
  esac
}
run_one() (
  set -e
  tool=$1
  data=$work/$tool
  "$BASHY" mkdir -p "$data"
  addr=127.0.0.1:0
  case "$tool" in
    notepad) export NOTEPAD_DATA=$data NOTEPAD_ADDR=$addr ;;
    form) export FORM_DATA=$data FORM_ADDR=$addr ;;
    chart) export CHART_DATA=$data CHART_ADDR=$addr ;;
    viewer) export VIEWER_DATA=$data VIEWER_ADDR=$addr ;;
    whiteboard) export WHITEBOARD_DATA=$data WHITEBOARD_ADDR=$addr ;;
    brainstorm) export BRAINSTORM_DATA=$data BRAINSTORM_ADDR=$addr ;;
  esac
  serve_tool "$tool" >"$data/server.log" 2>&1 &
  server_pid=$!
  trap 'kill "$server_pid" 2>/dev/null || true; wait "$server_pid" 2>/dev/null || true' EXIT
  ready=false
  for i in $(seq 1 120); do
    while IFS= read -r line; do
      case "$line" in Participant:\ http://*) origin=${line#Participant: }; break ;; esac
    done < "$data/server.log"
    if [[ -n ${origin:-} ]]; then
      origin=${origin%%\?*}
      origin=${origin%/}
      addr=${origin#http://}
      if "$BASHY" curl -fsS --max-time 1 "$origin/healthz" >/dev/null 2>&1; then ready=true; break; fi
    fi
    sleep 1
  done
  if [[ $ready != true ]]; then cat "$data/server.log" >&2; exit 1; fi
  case "$tool" in
    notepad) export NOTEPAD_ADDR=$addr ;;
    form) export FORM_ADDR=$addr ;;
    chart) export CHART_ADDR=$addr ;;
    viewer) export VIEWER_ADDR=$addr ;;
    whiteboard) export WHITEBOARD_ADDR=$addr ;;
    brainstorm) export BRAINSTORM_ADDR=$addr ;;
  esac
  check_json "$tool" "$("$BASHY" curl -fsS "$origin/healthz")"

  case "$tool" in
    notepad)
      printf '<h1>Imported</h1><p>From HTML</p>\n' > "$data/input.html"
      "$BASHY" lens -t md "$data/input.html" > "$data/import.md"
      created=$(cmd "$tool" add --title Imported -f "$data/import.md" --json)
      ;;
    form) created=$(cmd "$tool" add --poll 'Pick one' --options a,b,c --json) ;;
    chart)
      printf '[12,18,15,24]\n' > "$data/chart.json"
      created=$(cmd "$tool" add --input "$data/chart.json" --widget line --json)
      ;;
    viewer)
      printf '{"summary":"Round-trip status","done":["Built six tools"]}\n' > "$data/status.json"
      created=$(cmd "$tool" add --template status --data "$data/status.json" --json)
      ;;
    *) created=$(cmd "$tool" add --id desk-roundtrip --title 'Desk round-trip' --json) ;;
  esac
  id=$(field "$created" id)
  [[ -n $id && $id != null ]]
  key=$(field "$created" ownerKey)
  case "$tool" in
    form) export FORM_KEY=$key ;;
    chart) export CHART_KEY=$key ;;
    whiteboard) export WHITEBOARD_KEY=$key ;;
    brainstorm) export BRAINSTORM_KEY=$key ;;
  esac
  cmd "$tool" show "$id" --json >/dev/null
  case "$tool" in
    notepad) cmd "$tool" set "$id" --body '# Updated' --json >/dev/null ;;
    form) cmd "$tool" set "$id" --body 'Poll' --json >/dev/null ;;
    chart) cmd "$tool" set "$id" --input "$data/chart.json" --json >/dev/null ;;
    whiteboard|brainstorm) cmd "$tool" set "$id" --body 'Updated' --json >/dev/null ;;
  esac
  cmd "$tool" list --json >/dev/null
  cmd "$tool" open "$id" --json >/dev/null
  if [[ $tool == viewer ]]; then
    cmd "$tool" export "$id" --to html -o "$data/viewer.html" --json >/dev/null
    [[ -s $data/viewer.html ]]
  fi
  if [[ $tool == form ]]; then
    for participant in 1 2 3; do
      "$BASHY" curl -fsS -c "$data/cookie-$participant" -b "$data/cookie-$participant" "$origin/?id=$id" >/dev/null
    done
    for participant in 1 2 3; do
      case $participant in 1|3) vote=a ;; 2) vote=b ;; esac
      reply=$("$BASHY" curl -sS -c "$data/cookie-$participant" -b "$data/cookie-$participant" -H 'Content-Type: application/json' -d "{\"action\":\"answer\",\"id\":\"$id\",\"complete\":true,\"answers\":{\"vote\":\"$vote\"}}" "$origin/api/action")
      check_json form "$reply"
      printf '%s\n' "$reply" | "$BASHY" jq -e '.error == null' >/dev/null || { echo "form vote refused: $reply" >&2; exit 1; }
    done
    shown=$(cmd form show "$id" --json)
    printf '%s\n' "$shown" | "$BASHY" jq -e '.results.aggregate.vote.a == 2 and .results.aggregate.vote.b == 1' >/dev/null
    reply=$("$BASHY" curl -sS -c "$data/cookie-1" -b "$data/cookie-1" -H 'Content-Type: application/json' -d "{\"action\":\"answer\",\"id\":\"$id\",\"complete\":true,\"answers\":{\"vote\":\"c\"}}" "$origin/api/action")
    check_json form "$reply"
    printf '%s\n' "$reply" | "$BASHY" jq -e '.error == null' >/dev/null || { echo "form re-vote refused: $reply" >&2; exit 1; }
    shown=$(cmd form show "$id" --json)
    printf '%s\n' "$shown" | "$BASHY" jq -e '.results.aggregate.vote.a == 1 and .results.aggregate.vote.b == 1 and .results.aggregate.vote.c == 1' >/dev/null
  fi
  rendered=$("$BASHY" curl -fsS -H 'Content-Type: application/json' -d '{"text":"# Desk\n\n<script>alert(1)</script>"}' "$origin/api/render")
  check_json "$tool" "$rendered"
  printf '%s\n' "$rendered" | "$BASHY" jq -e '.html | contains("desk-content") and (contains("<script") | not)' >/dev/null
  if [[ $tool == notepad ]]; then
    rendered=$("$BASHY" curl -fsS -H 'Content-Type: application/json' -d '{"text":"```chart widget=bar\nname,value\na,1\nb,2\n```"}' "$origin/api/render")
    check_json notepad "$rendered"
    printf '%s\n' "$rendered" | "$BASHY" jq -e '.html | contains("<svg")' >/dev/null
  fi
  cmd "$tool" close "$id" --json >/dev/null
  if [[ $tool == form ]]; then
    status=$("$BASHY" curl -sS -o "$data/refusal.json" -w '%{http_code}' -c "$data/cookie-2" -b "$data/cookie-2" -H 'Content-Type: application/json' -d "{\"action\":\"answer\",\"id\":\"$id\",\"complete\":true,\"answers\":{\"vote\":\"a\"}}" "$origin/api/action")
    [[ $status == 409 ]]
    check_json form "$(cat "$data/refusal.json")"
  fi
  set +e
  refused=$(cmd "$tool" unsupported-desk-verb --json)
  code=$?
  set -e
  [[ $code == 2 ]]
  printf '%s\n' "$refused" | "$BASHY" jq -e '.error.code | length > 0' >/dev/null
  printf '%s\n' "$refused" | "$BASHY" jq -e '.error.message | length > 0' >/dev/null
)

for tool in notepad form chart viewer whiteboard brainstorm; do
  start=$(date +%s)
  # A background child keeps `set -e` effective inside run_one; calling a
  # function directly in an `if` would silently disable it in Bash.
  run_one "$tool" & run_pid=$!
  if wait "$run_pid"; then result=pass; else result=fail; failed=1; fi
  duration=$(($(date +%s)-start))
  printf '%s\n' "$tool $result ${duration}s"
  printf '{"tool":"%s","result":"%s","duration_seconds":%s}\n' "$tool" "$result" "$duration" >> "$results"
done
printf '{"schema_version":"bashy-desk-roundtrip-v1","os":"%s","arch":"%s","bashy_version":"%s","tools":[' "$os" "$arch" "$version" > "$evidence"
first=true
while IFS= read -r record; do
  if [[ $first == false ]]; then printf ',' >> "$evidence"; fi
  first=false
  printf '%s' "$record" >> "$evidence"
done < "$results"
printf ']}\n' >> "$evidence"
if [[ $failed == 0 ]]; then "$BASHY" rm -rf "$work"; else echo "roundtrip diagnostics: $work" >&2; fi
exit "$failed"
```

### gallery
Capture light and dark screenshots of every desk tool when Playwright and a
browser are available. A missing browser is reported as a skip.
Requires: build
Effects: read, write, exec, net

```bash
set -euo pipefail
export BASHY=${BASHY:-bashy}
root=$(git rev-parse --show-toplevel)
if [[ -n ${PLAYWRIGHT_MODULE:-} && ! -f $PLAYWRIGHT_MODULE ]]; then
  echo 'SKIP gallery: PLAYWRIGHT_MODULE does not exist'
  exit 0
fi
if ! "$BASHY" awd "$root/examples/desk/gallery" -- node -e 'import("node:url").then(u => import(process.env.PLAYWRIGHT_MODULE ? u.pathToFileURL(process.env.PLAYWRIGHT_MODULE).href : "playwright")).then(async m => {const fs = await import("node:fs"); process.exit(fs.existsSync(process.env.CHROMIUM_PATH || m.chromium.executablePath()) ? 0 : 1)}).catch(() => process.exit(1))'; then
  echo 'SKIP gallery: Playwright or its Chromium browser is unavailable on this host'
  exit 0
fi
bins=$("$BASHY" mktemp -d)
trap '"$BASHY" rm -rf "$bins"' EXIT
for tool in notepad form chart viewer; do
  case $tool in form) src=$root/examples/desk/form/server ;; *) src=$root/examples/desk/$tool ;; esac
  GOWORK=off CGO_ENABLED=0 "$BASHY" awd "$src" -- go build -o "$bins/$tool" .
done
for tool in whiteboard brainstorm; do
  GOWORK=off CGO_ENABLED=0 "$BASHY" awd "$root/examples/$tool/server" -- go build -o "$bins/$tool" .
done
DESK_BIN=$bins "$BASHY" awd "$root/examples/desk/gallery" -- node capture.mjs
```

### evidence
Print and validate the single record written by `roundtrip`.
Requires: roundtrip
Effects: read

```bash
set -euo pipefail
"${BASHY:-bashy}" jq -e '.schema_version == "bashy-desk-roundtrip-v1" and (.tools | length) == 6 and ([.tools[].result] | all(. == "pass"))' "${EVIDENCE:-./desk-roundtrip.json}"
```
