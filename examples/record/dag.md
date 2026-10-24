---
name: record
description: tutorial — build, test, register, show and use a skill-tier bashy command (record) written as one Bash# file that carries its own skill
type: dag
default: test
vars:
  - NAME ?= record
---

# record — the skill-tier command cycle, as a tutorial

One file, `record.bsh`, is both the skill an agent reads (its `~~~skill`
fence) and the program bashy runs (a Go fence for the loopback server plus a
few lines of shell). This graph is the whole cycle any agent follows to add a
command without touching bashy's core:

    test  ->  register  ->  show  ->  use

Run it by naming the folder: `bashy dag examples/record <target>`.

## Tasks

### test
Fast checks that need no camera: help, the embedded skill, a refused kind
(exit 1), a refused remote session.
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, exec

```bash
set -e
bashy --bashsharp record.bsh --help | head -1 | grep -q '^record'
bashy --bashsharp record.bsh --skill | grep -q '^name: record$'
if bashy --bashsharp record.bsh hologram 2>/dev/null; then echo "bad kind accepted" >&2; exit 1; fi
if SSH_CONNECTION="10.0.0.1 1 10.0.0.2 22" bashy --bashsharp record.bsh photo --timeout 2 2>/dev/null; then echo "remote session accepted" >&2; exit 1; fi
echo "test: ok"
```

### register
Register `record.bsh` as the command NAME: typed arguments (the MCP tool
schema), the skill as its help, and its effects. Re-running replaces it.
Requires: test
Env: BASHY_HINTS=off, BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec

```bash
set -e
here=$(pwd)
long=$(bashy --bashsharp record.bsh --skill)
bashy commands rm "$NAME" >/dev/null 2>&1 || true
bashy commands add "$NAME" \
  --set exec.0=bashy --set exec.1=--bashsharp --set exec.2="$here/record.bsh" \
  --set synopsis="get a photo, video or voice take from the person at this computer (camera page with teleprompter); prints the path" \
  --set long="$long" \
  --set group=platform \
  --set effects.0=read --set effects.1=write --set effects.2=exec --set effects.3=net \
  --set args.positionals.0.name=kind \
  --set args.positionals.0.enum.0=photo --set args.positionals.0.enum.1=video --set args.positionals.0.enum.2=audio \
  --set args.flags.0.name=prompt \
  --set args.flags.1.name=script \
  --set args.flags.2.name=max-seconds --set args.flags.2.type=int --set args.flags.2.default=180 \
  --set args.flags.3.name=out \
  --set args.flags.4.name=timeout --set args.flags.4.type=int --set args.flags.4.default=1200 \
  --set args.flags.5.name=lang --set args.flags.5.default=en-US
echo "register: $NAME -> $here/record.bsh"
```

### show
What any agent sees: the catalog line, the skill as help, the MCP tool schema.
Requires: register
Env: BASHY_HINTS=off
Effects: read

```bash
bashy commands | grep -w "$NAME" || true
bashy commands "$NAME" | head -20
bashy help "$NAME" --format=mcp | head -40
```

### use
Ask the person for a photo (they allow the camera and press the buttons).
Requires: register
Env: BASHY_TELEMETRY_QUIET=1
Effects: read, write, exec, net

```bash
path=$("$NAME" photo --prompt "Test photo: press Take photo, then Use this")
echo "use: $path"
ls -l "$path"
```
