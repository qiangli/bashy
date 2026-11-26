---
name: notepad
description: Lifecycle of the notepad shapeware app (source, build, test, install, run, serve, stop). Use when asked to fetch, build, test, install, run, serve or stop notepad.
type: dag
default: serve
---

# notepad: lifecycle

Desk notepad: view any content as text (txt, md, html, json, PDF->text),
edit Markdown in the loopback page, agent reads/puts content, lists and shares.

Every step is exact and non-interactive. Run one from this folder with
`bashy dag -f dag.md <step>` (or `bashy awd PATH -- bashy dag -f dag.md <step>`).
Without bashy, read the step's code block. `$BASHY` is set by `bashy dag`.
The lifecycle of the app as a spec lives in `spec/dag.md` and runs with `sh`.

## Tasks

### source
Where the source is. This folder is the complete source; nothing to fetch.
Effects: read
Band: none

```bash
echo "source: this folder (notepad); nothing to fetch"
```

### build
Build the Go server/CLI (writes `./notepad-bin`, run by the `./notepad` launcher).
Effects: read, write, exec
Band: unmeasured

```bash
sh ./build.sh
```

### test
Run the desk contract test and unit tests.
Effects: read, write, exec
Band: none

```bash
"$BASHY" ./notepad.sh test
```

### install
Install `notepad` as a registered bashy command, shapeware skill, and shaped app.
Effects: read, write, exec
Band: unmeasured

```bash
"$BASHY" ./register.sh
```

### run
Run `notepad` with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='add --id note-1 --title "Probe" --body "Hello" --json'`. Default: `list`. The server must be running first (see `serve`).
Effects: read, write, exec, net
Band: unmeasured

```bash
mapfile -d '' -t argv < <(printf '%s' "${ARGS:-list}" | xargs printf '%s\0')
"$BASHY" ./notepad.sh "${argv[@]}"
```

### serve
Build and start the loopback-only notepad server (default 127.0.0.1:18095).
Effects: read, write, exec, net
Band: unmeasured

```bash
"$BASHY" ./notepad.sh serve
```

### stop
Stop this user's notepad server, whatever started it. Exits 0 when none is running.
Effects: read, exec
Band: unmeasured

```bash
./notepad stop 2>/dev/null || true
pkill -f "desk-notepad/notepad serve" 2>/dev/null || true
echo "stopped notepad"
```

### register
Project the notepad into an ordinary command record, skill and launcher tile.
Effects: read, write, exec
Band: unmeasured

```bash
"$BASHY" ./register.sh
```

### show
Read the registered command and skill face.
Effects: read
Band: unmeasured

```bash
"$BASHY" commands show notepad
"$BASHY" skill show notepad
```
