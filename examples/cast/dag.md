---
name: cast
description: build, test, register and use the cast skill-tier command (asciicast v2 recorder over pty/ConPTY) written as one Bash# file
type: dag
default: test
vars:
  - NAME ?= cast
---

# cast — the skill-tier command cycle

One file, `cast.bsh`, is both the skill an agent reads (its `~~~skill` fence)
and the program bashy runs (a Go fence over the `./cast` package, built
against `./go.mod`). Run it by naming the folder: `bashy dag examples/cast
<target>`.

## Tasks

### test
The recorder package's tests (colour codes, a multi-byte rune split across
reads, idle limit, exit status) and the script driven end to end, plus the
Windows cross-build of the ConPTY side.
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1, GOWORK=off
Effects: read, write, exec, net

```bash
set -e
go test ./...
GOOS=windows GOARCH=amd64 go build ./...
bashy --bashsharp cast.bsh --help | head -1 | grep -q '^cast'
bashy --bashsharp cast.bsh --skill | grep -q '^name: cast$'
echo "test: ok"
```

### register
Register `cast.bsh` as the command NAME: the skill as its help, its effects
and its argument shape. Re-running replaces it.
Requires: test
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec

```bash
set -e
here=$(pwd)
long=$(bashy --bashsharp cast.bsh --skill)
bashy commands rm "$NAME" >/dev/null 2>&1 || true
bashy commands add "$NAME" \
  --set exec.0=bashy --set exec.1=--bashsharp --set exec.2="$here/cast.bsh" \
  --set synopsis="record a command's terminal output as asciicast v2 under a pty/ConPTY; exit status is the command's" \
  --set long="$long" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec
echo "registered: $NAME -> $here/cast.bsh"
bashy "$NAME" --help | head -1
```

### use
Record one scene into the current directory and print its header.
Requires: register
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec

```bash
set -e
bashy "$NAME" record scene.cast --cols 60 --rows 12 -- sh -c 'printf "\033[32mhello from cast\033[0m\n"'
head -1 scene.cast
```
