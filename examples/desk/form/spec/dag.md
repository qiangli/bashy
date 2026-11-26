---
name: form
description: Lifecycle of the form app when it is delivered as a spec (source, build, test, install, run, serve, stop). Use when asked to build, test, install, run, serve or stop form.
type: dag
default: test
---

# form: lifecycle

This file is part of the spec, like CONTRACT.md. An implementation makes
every step below work; it does not change this file. Every step is exact and
non-interactive. Run one by running its code block with `sh` from this
folder. Nothing here needs bashy. With bashy, `bashy dag -f dag.md <step>`
does the same.

The app is `./form`, an executable POSIX-sh launcher in this folder.
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
Produce what runs. `build.sh` is supplied by the implementation: it compiles or prepares everything `./form` needs, writes only inside this folder, needs no network, and can be run again safely. With nothing to compile it says so and exits 0.
Effects: read, write, exec
Band: unmeasured

```sh
sh ./build.sh
```

### test
Run every assertion of the contract against `./form`, each in a fresh HOME. Exit 1 on any FAIL.
Effects: read, write, exec, net
Band: none

```sh
conform run CONTRACT.md --path . --fresh-home
```

### install
Install `form` as a command for this user: a launcher in `$PREFIX/bin` (default `$HOME/.local/bin`) that runs this folder's `./form`. The folder stays where it is; the directory must be on the PATH for `form` to be found.
Effects: read, write
Band: unmeasured

```sh
bin=${PREFIX:-$HOME/.local}/bin
mkdir -p "$bin"
printf '#!/bin/sh\nexec "%s/form" "$@"\n' "$(pwd -P)" > "$bin/form"
chmod +x "$bin/form"
echo "installed: $bin/form"
```

### run
Run `form` with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='add --def examples/contact.json --title "Intake" --json'`.
Effects: read, write, exec, net
Band: unmeasured

```sh
: "${ARGS:?set ARGS, for example ARGS='add --def examples/contact.json --json'}"
printf '%s' "$ARGS" | xargs ./form
```

### serve
Serve the pages of this user's forms on the loopback address, in the foreground, for a service that something else supervises. Any server of this user that is already running is stopped first. While it runs, the `url` of every form answers. Stop it with the `stop` step.
Effects: read, write, exec, net
Band: unmeasured

```sh
./form stop
exec ./form serve
```

### stop
Stop this user's form server, whatever started it. Exits 0 when none is running. Stored forms are kept.
Effects: read, exec
Band: unmeasured

```sh
./form stop
```
