---
name: notepad
description: Lifecycle of the notepad app when it is delivered as a spec (source, build, test, install, run, serve, stop). Use when asked to build, test, install, run, serve or stop notepad.
type: dag
default: test
---

# notepad: lifecycle

This file is part of the spec, like CONTRACT.md. An implementation makes
every step below work; it does not change this file. Every step is exact and
non-interactive. Run one by running its code block with `sh` from this
folder. Nothing here needs bashy. With bashy, `bashy dag -f dag.md <step>`
does the same.

The app is `./notepad`, an executable POSIX-sh launcher in this folder.
The judge is `conform`, on the PATH.

## Tasks

### source
Where the source is. This folder is the complete source; nothing to fetch.
Effects: read
Band: none

```sh
echo "source: this folder; nothing to fetch"
```

### build
Produce what runs. `build.sh` is supplied by the implementation: it compiles or prepares everything `./notepad` needs, writes only inside this folder, needs no network, and can be run again safely. With nothing to compile it says so and exits 0.
Effects: read, write, exec
Band: unmeasured

```sh
sh ./build.sh
```

### test
Run every assertion of the contract against `./notepad`. Exit 1 on any FAIL. The contract starts and stops its own server, and `conform cert` needs `--dir` set to this folder so the `examples/` files resolve.
Effects: read, write, exec, net
Band: none

```sh
conform cert --contract CONTRACT.md --reference-cmd "$PWD/notepad" --candidate-cmd "$PWD/notepad" --runs 3 --dir "$PWD" --fresh-home
```

### install
Install `notepad` as a command for this user: a launcher in `$PREFIX/bin` (default `$HOME/.local/bin`) that runs this folder's `./notepad`. The folder stays where it is; the directory must be on the PATH for `notepad` to be found.
Effects: read, write
Band: unmeasured

```sh
bin=${PREFIX:-$HOME/.local}/bin
mkdir -p "$bin"
printf '#!/bin/sh\nexec "%s/notepad" "$@"\n' "$(pwd -P)" > "$bin/notepad"
chmod +x "$bin/notepad"
echo "installed: $bin/notepad"
```

### run
Run `notepad` with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='--addr 127.0.0.1:18195 --data ./.npdata list --json'`. The server must be running first (see `serve`).
Effects: read, write, exec, net
Band: unmeasured

```sh
: "${ARGS:?set ARGS, for example ARGS='list --json'}"
printf '%s' "$ARGS" | xargs ./notepad
```

### serve
Serve this user's notes on the loopback address, in the foreground, for a service that something else supervises. Any server of this user that is already running is stopped first. While it runs, the `url` of every note answers. Stop it with the `stop` step.
Effects: read, write, exec, net
Band: unmeasured

```sh
./notepad stop
ADDR=${ADDR:-127.0.0.1:18195}
DATA=${DATA:-./.npdata}
exec ./notepad --addr "$ADDR" --data "$DATA" serve
```

### stop
Stop this user's notepad server, whatever started it. Exits 0 when none is running. Stored notes are kept.
Effects: read, exec
Band: unmeasured

```sh
./notepad stop
```
