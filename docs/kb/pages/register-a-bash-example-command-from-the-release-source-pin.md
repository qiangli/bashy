---
id: 01a1b809-6411-749b-8b3e-d1a0e0df3082
seq: 14
form: page
type: runbook
title: Register a Bash# example command from the release source pin (one .bsh = command + skill)
description: 'Find and register Bashy''s shipped Bash# example scripts as commands with bashy src fetch + bashy commands add, so the script and its embedded skill guide match the installed release — WHEN a runbook step is already covered by an example script, or you want an example (outpost-profile, ...) as a command (keywords: bash#, bsh, bashsharp, commands add, src fetch, skill as guide, example, register command)'
tags:
    - bashsharp
    - commands
    - skills
    - examples
status: candidate
evidence: '2026-11-07: outpost-profile registered this way from the rc.27 source pin and run live; flags per bashy commands add --help'
source:
    tool: claude
created: "2026-11-07T22:03:05Z"
---

## Why

A Bash# example ships in Bashy's source as ONE `.bsh` file that is both the executable command and its skill guide: the guide sits in a `~~~skill as guide` fence inside the script (see `examples/outpost-profile/outpost-profile.bsh`). Registering it gives a normal command, and the agent-facing guide travels with it. Before writing a runbook in prose, check whether such a script already covers the steps.

## Find what exists

```sh
bashy commands --view registered     # commands registered on this host
bashy skill list                     # skills, including registered .bsh guides
ls "${BASHY_HOME:-$HOME/.bashy}/src/bashy/examples"   # after a fetch (below)
```

## Register an example from the release source pin

`bashy src fetch` fetches a repo at the running binary's release commit, so the script matches the installed bashy:

```sh
bashy src fetch bashy
NAME=outpost-profile
bashy commands add "$NAME" \
  --set exec.0=bashy --set exec.1=--bashsharp \
  --set "exec.2=${BASHY_HOME:-$HOME/.bashy}/src/bashy/examples/$NAME/$NAME.bsh" \
  --set effects.0=read --set effects.1=write --set effects.2=exec
"$NAME" --help
```

- Declare the effects the script really has (read / write / exec / net ...).
- A name that already belongs to a shipped command is refused (no override); `--force` only takes over another registered command.
- After `bashy src upgrade bashy` the registration keeps pointing at the same path, now with the newer script.
- To change a script: `bashy src branch bashy <topic>`, edit, run its Go test (e.g. `go test ./examples/outpost-profile/`), and register that path.
