---
name: shape
description: Lifecycle and gate targets for the shape surface. Use to resolve, build, test, install, run or certify the shape surface.
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
if [ -n "$top" ] && [ "$top" != "$(pwd -P)" ] && [ -f "$top/shapeware/index.bsh" ]; then
    impl=$top
else
    impl=$(pwd -P)/.shapeware/src
    [ -d "$impl/.git" ] || git clone -q https://github.com/qiangli/bashy.git "$impl"
    git -C "$impl" checkout -q --detach bb467707bd5f0f7f269aab4e5ff9c188c7c00d23
fi
printf '%s\n' "$impl" > .shapeware/impl
echo "implementation: $impl"
```

### build
Build bashy from the resolved implementation on the gate host and write a `shape` launcher (a shim that execs `bashy shape`) beside it, so the contract's `shape` commands run the built binary.
Effects: read, write, exec, net, priv
Band: unmeasured

```bash
set -eu
here=$(pwd -P)
impl=${IMPL:-$(cat .shapeware/impl)}
mkdir -p .shapeware/bin
(cd "$impl" && GOWORK=off CGO_ENABLED=0 go build -o "$here/.shapeware/bin/bashy" ./cmd/bashy)
printf '%s\n' '#!/bin/sh' 'exec "$(dirname "$0")/bashy" shape "$@"' > .shapeware/bin/shape
chmod +x .shapeware/bin/shape
.shapeware/bin/bashy --version
```

### test
Run the contract once against the built shape launcher with a fresh HOME on the gate host.
Effects: read, write, exec
Band: none

```bash
sh ./judge.sh test "$(pwd -P)/.shapeware/bin"
```

### install
Install this spec folder as a bashy skill for the current user.
Effects: read, write, exec
Band: unmeasured

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
"$(pwd -P)/.shapeware/bin/bashy" skill add .
echo "installed: shape skill from $(pwd -P)"
```

### run
Run the shape surface with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='status hello-world --json'`.
Effects: read, write, exec, net
Band: unmeasured

```bash
set -eu
impl=${IMPL:-$(cat .shapeware/impl)}
: "${ARGS:?set ARGS, for example ARGS='status hello-world --json'}"
printf '%s' "$ARGS" | xargs "$(pwd -P)/.shapeware/bin/bashy" shape
```

### check
Lint this folder and, in the implementation repository, run the shapeware index check. Runs anywhere.
Effects: read
Band: none

```bash
set -eu
runner=${BASHY:-bashy}
"$runner" conform spec check .
top=$(git rev-parse --show-toplevel 2>/dev/null || true)
if [ -n "$top" ] && [ -f "$top/shapeware/index.bsh" ]; then
    "$runner" --bashsharp "$top/shapeware/index.bsh" check
fi
```

### cert
Certify the built reference on the gate host in three runs with a fresh HOME per run. Set CANDIDATE to a command that implements the shape CLI to grade it against this reference.
Effects: read, write, exec
Band: none

```bash
sh ./judge.sh cert "$(pwd -P)/.shapeware/bin" "${CANDIDATE:-}"
```

### mutate
Mutation-test the built launcher on the gate host. Budget 122 default CLI mutants times one 32-assertion contract run.
Effects: read, write, exec
Band: none

```bash
sh ./judge.sh mutate "$(pwd -P)/.shapeware/bin"
```

### alone
Copy the standalone spec outside the checkout and run its source, build and test steps on the gate host. Fresh-agent review is a separate review step.
Effects: read, write, exec, net, priv
Band: none

```bash
set -eu
scratch=$(mktemp -d /tmp/shapeware-shape-alone.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT
mkdir "$scratch/shape"
cp -R SKILL.md CONTRACT.md features.yaml dag.md judge.sh examples "$scratch/shape/"
cd "$scratch/shape"
unset IMPL
bashy dag -f dag.md source
bashy dag -f dag.md build
bashy dag -f dag.md test
```
