---
name: agent-identity
description: Lifecycle and gate targets for the agent-identity surface. Use to resolve, build, test, install, run or certify the agent-identity surface.
type: dag
default: test
---

# agent-identity: lifecycle

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
    git -C "$impl" checkout -q --detach a5b5a943502db5795e3c699b23ad64d185f76a63
fi
printf '%s\n' "$impl" > .shapeware/impl
echo "implementation: $impl"
```

### build
Build bashy from the resolved implementation on the gate host and write the `session` launcher beside it: `session KIND VERB ARGS` runs the built `bashy VERB ARGS` as the caller `KIND` names (the table in `CONTRACT.md`), and `session gotest PKG NAME` runs one pinned go test of the implementation through the reusable lifted-surface launcher.
Effects: read, write, exec, net, priv
Band: unmeasured

```bash
set -eu
here=$(pwd -P)
impl=${IMPL:-$(cat .shapeware/impl)}
mkdir -p .shapeware/bin
(cd "$impl" && GOWORK=off CGO_ENABLED=0 go build -o "$here/.shapeware/bin/bashy" ./cmd/bashy)
cat > .shapeware/bin/session <<'LAUNCHER'
#!/bin/sh
# session KIND VERB ARGS: run the built bashy as the caller KIND names.
set -eu
[ $# -ge 2 ] || { echo "usage: session KIND VERB [ARGS]" >&2; exit 2; }
kind=$1; shift
bashy=$(dirname "$0")/bashy
case $kind in
    gotest)           exec env LAUNCHER_IMPL="$(cat "$(dirname "$0")/impl")" sh "$(dirname "$0")/launcher.sh" gotest "$@" ;;
    human)            exec "$bashy" "$@" ;;
    claude:?*)        exec env CLAUDECODE=1 CLAUDE_CODE_SESSION_ID="${kind#claude:}" "$bashy" "$@" ;;
    codex:?*)         exec env CODEX_THREAD_ID="${kind#codex:}" "$bashy" "$@" ;;
    unnamed)          exec env GEMINI_CLI=1 "$bashy" "$@" ;;
    unnamed+name)     exec env GEMINI_CLI=1 BASHY_AGENT=my-own-name "$bashy" "$@" ;;
    unnamed+off)      exec env GEMINI_CLI=1 BASHY_ADHOC_IDENTITY=off "$bashy" "$@" ;;
    *)                echo "session: unknown kind: $kind" >&2; exit 2 ;;
esac
LAUNCHER
chmod +x .shapeware/bin/session
cp "$impl/examples/shapeware/lifted-surface/scripts/launcher.sh" .shapeware/bin/launcher.sh
printf '%s\n' "$impl" > .shapeware/bin/impl
.shapeware/bin/bashy --version
```

### test
Run the contract once against the built session launcher with a fresh HOME on the gate host.
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
echo "installed: agent-identity skill from $(pwd -P)"
```

### run
Run one bashy verb as a named kind of caller with `ARGS`, split with shell quoting and never evaluated. Example: `ARGS='claude:session-one claim list'`.
Effects: read, write, exec, net
Band: unmeasured

```bash
set -eu
: "${ARGS:?set ARGS, for example ARGS='claude:session-one claim list'}"
printf '%s' "$ARGS" | xargs "$(pwd -P)/.shapeware/bin/session"
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
Certify the built reference on the gate host in three runs with a fresh HOME per run. Set CANDIDATE to a command that implements the `session` launcher to grade it against this reference.
Effects: read, write, exec
Band: none

```bash
sh ./judge.sh cert "$(pwd -P)/.shapeware/bin" "${CANDIDATE:-}"
```

### mutate
Mutation-test the built launcher on the gate host. Budget the default CLI mutants times one 23-assertion contract run (about a minute with warm caches; three assertions are cited go tests).
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
scratch=$(mktemp -d /tmp/shapeware-agent-identity-alone.XXXXXXXX)
trap 'rm -rf "$scratch"' EXIT
mkdir "$scratch/agent-identity"
cp -R SKILL.md CONTRACT.md features.yaml dag.md judge.sh examples "$scratch/agent-identity/"
cd "$scratch/agent-identity"
unset IMPL
bashy dag -f dag.md source
bashy dag -f dag.md build
bashy dag -f dag.md test
```
