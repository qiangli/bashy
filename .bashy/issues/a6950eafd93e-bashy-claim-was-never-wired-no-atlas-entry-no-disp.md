---
id: a6950eafd93e
kind: bug
title: 'bashy claim was never wired: no atlas entry, no dispatch, and the refusal points at a nonexistent `bashy claims`'
status: closed
stage: code
priority: p1
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T22:55:18.511712Z
closed: 2026-07-13T00:02:14.075436Z
resolution: fixed
closed_by: codex-l
---

## Symptom

    $ bashy claim
    bashy: claim: command not found

`coreutils/pkg/policy/coord` exists, is tested, and its GUARD works (a second agent
writing the same project IS refused — verified live). But the VERB was never
registered:

  - no `addVerb("claim", ...)` in `coreutils/pkg/atlas/atlas.go`
  - no `case "claim":` in `bashy/internal/agentos/agentos.go`
  - not in `alwaysShimVerbs`
  - `coord.NewClaimCmd()` has zero callers

## The worse half

The refusal message an agent sees tells it to run a command that does not exist
(`coord.go:241`):

    bashy claims                    # who is working, where, on what

So the one affordance that makes the guard survivable — "who is holding this, and
how do I see it?" — is a dead end. An agent that hits the guard is told to run
something that fails.

## Why the ratchet missed it

`TestE2EAllListedCommandsDispatch` iterates what `bashy commands` ADVERTISES. A verb
that was never advertised is invisible to it. The gate catches "advertised but
broken", not "built but never advertised". Consider closing that gap too: assert that
every `NewXxxCmd` constructor exported from a pkg/ with a front-door intent has an
atlas entry — or at minimum, that `coord.NewClaimCmd` is reachable.

## Task

1. Register `claim` in the atlas (stage: cross, group: orchestration; it READS and
   WRITES `~/.bashy/coord/`).
2. Dispatch `case "claim":` in agentos, calling `coord.NewClaimCmd(...)` with a roots
   func (see how `issue` passes `detectProjectRoot`; claim wants
   `handoff.ProjectRoots(projectRootOf(cwd))` — the PATH SET, not the git root).
3. Add "claim" to `alwaysShimVerbs`.
4. Make the refusal message name the command that actually exists.
5. Add a test that `bashy claim`, `bashy claim list`, `bashy claim release` all run.

## Verify

    go test ./internal/agentos/ && go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos
    bin/bashy claim list
