---
name: clip
description: build, test, register and use the clip skill-tier command (async screen and video clips) written as one Bash# file over a small Go engine
type: dag
default: test
vars:
  - NAME ?= clips
---

# clip — the skill-tier command cycle

`clip.bsh` is the skill an agent reads (its `~~~skill` fence) and the
program bashy runs (a cached build of `./server`), over this example's own
standard-library-only module. The engine is a file-based job store, a
render queue and one loopback library page. Run it from anywhere by naming
the folder: `bashy dag examples/clip <target>`.

## Tasks

### test
The engine's tests (job store round-trip and validation, the worker with
injected tools for trim/cast/screen and failure paths, the page and its
media route, the shaped.json contract) plus the script driven end to end
through a built bashy with a stub ffmpeg.
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1, GOWORK=off
Effects: read, write, exec, net

```bash
set -e
go test ./...
echo "test: ok"
```

### register
Register `clip.bsh` as the command NAME (skill as its help, effects,
exec shape), add the executable skill, and register the shaped app
through the public `bashy app add --type shaped` path. Re-running replaces.
Requires: test
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec

```bash
set -e
BASHY_BIN="${BASHY_BIN:-bashy}" bash register.sh "$NAME"
```

### use
Queue and render the sample terminal scene with the host's real tools
(agg, ffmpeg), then take the finished clip's path. Needs agg on PATH;
skips itself when the host has none.
Requires: test
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec

```bash
set -e
command -v agg >/dev/null || { echo "use: agg not installed; skipping the real render"; exit 0; }
data=$(mktemp -d)
scene=$(pwd)/sample/scene.cast
out=$(bashy --bashsharp clip.bsh queue --from "$scene" --start 1 --end 3 --name sample --data "$data" | head -1)
bashy --bashsharp clip.bsh work --data "$data" --verbose
bashy --bashsharp clip.bsh status --data "$data"
clip_file=$(bashy --bashsharp clip.bsh take "$out" --data "$data")
[ -s "$clip_file" ]
echo "use: $clip_file"
```
