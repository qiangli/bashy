---
name: shape
description: Lifecycle of the shape lifted surface (source, build, test, install, run). Use when asked to fetch, build, test, install or run the shape surface.
type: dag
default: test
---

# shape: lifecycle

Every step is exact and non-interactive. Run one from this folder with `bashy dag -f dag.md <step>`; without bashy, read the step's code block. `$BASHY` is set by `bashy dag`. Builds and behavioural tests run only on a gate host with sandbox fakes, never on a shared development machine.

## Tasks

### source
Resolve the implementation: the containing repository when this folder sits in it, else a clone at the certified commit. Records the path in `.shapeware/impl`; later steps read `${IMPL:-$(cat .shapeware/impl)}`.
Effects: read, write, net
Band: none

```bash
set -eu
mkdir -p .shapeware
top=$(git rev-parse --show-toplevel 2>/dev/null || true)
if [ -n "$top" ] && [ "$top" != "$PWD" ] && [ -f "$top/shapeware/index.bsh" ]; then
    impl=$top
else
    impl=$PWD/.shapeware/src
    [ -d "$impl/.git" ] || git clone -q https://github.com/qiangli/bashy.git "$impl"
    git -C "$impl" checkout -q --detach bb467707bd5f0f7f269aab4e5ff9c188c7c00d23
fi
printf '%s\n' "$impl" > .shapeware/impl
echo "implementation: $impl"
```

### build
Build bashy from the resolved implementation on the gate host and write a `shape` launcher (a shim that execs `bashy shape`) beside it, so the contract's `shape` commands run the built binary.
Effects: read, write, exec
Band: unmeasured

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
mkdir -p .shapeware/bin
(cd "$impl" && GOWORK=off CGO_ENABLED=0 go build -o "$OLDPWD/.shapeware/bin/bashy" ./cmd/bashy)
printf '#!/bin/sh\nexec "%s" shape "$@"\n' "$PWD/.shapeware/bin/bashy" > .shapeware/bin/shape
chmod +x .shapeware/bin/shape
.shapeware/bin/bashy --version
```

### test
Certify the contract against the built `shape` launcher (3 runs, none noisy) on the gate host with a fresh HOME per run.
Effects: read, write, exec
Band: none

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
"$PWD/.shapeware/bin/bashy" conform cert --contract CONTRACT.md --reference-cmd "$PWD/.shapeware/bin/shape" --candidate-cmd "$PWD/.shapeware/bin/shape" --runs 3 --dir "$PWD" --fresh-home
```

### install
Install this spec folder as a bashy skill for the current user.
Effects: read, write, exec
Band: unmeasured

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
"$PWD/.shapeware/bin/bashy" skill add .
echo "installed: shape skill from $PWD"
```

### run
Run the shape surface with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='status hello-world --json'`.
Effects: read, write, exec, net
Band: unmeasured

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
: "${ARGS:?set ARGS, for example ARGS='status hello-world --json'}"
printf '%s' "$ARGS" | xargs "$PWD/.shapeware/bin/bashy" shape
```
