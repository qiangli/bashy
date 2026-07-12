---
id: 8d59842913ec
kind: bug
title: weave marks a KILLED, zero-commit run as 'submitted' — an empty branch is mergeable and auto-closes its issue as fixed
status: open
priority: p0
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T23:12:15.575382Z
---

## Observed, live

Run #10 (agy) hung on an interactive prompt, was TERMINATED by the idle watchdog, and
produced ZERO commits. weave recorded:

    state:    submitted
    exit:     0
    killed:   idle 12m27s exceeds --idle-timeout 10m0s
    commits:  0 commit(s) ahead of main

`submitted` means "the subagent exited cleanly and its branch is ready to be merged by
`weave pull`". Neither clause is true here.

## Why this is P0

`weave pull` would merge an EMPTY branch. And `weaveCloseRegisterOnMerge` would then
close the register entry with `resolution: fixed` -- auto-closing a bug that was never
touched, by an agent that never ran.

An agent that did NOTHING is currently indistinguishable from an agent that SUCCEEDED.
That is the precise failure the gate/judge split exists to prevent, reappearing inside
the machinery itself.

## Root cause

The terminal-state transition reads only the exit code. A watchdog kill (SIGTERM) lets
the process exit 0, so `exit == 0` => "submitted". `KilledBy` is RECORDED but not
CONSULTED.

## Fix

Two independent guards; both are needed, neither is sufficient alone:

1. If `KilledBy != ""` the run was NOT submitted by the agent -- it was stopped. State
   must be `killed`, regardless of exit code. (An agent killed at max-runtime after real
   work still has its branch; `weave salvage` is the path for that, and it already
   exists precisely so a killed run's work is recovered DELIBERATELY.)

2. A run with ZERO commits ahead of base has nothing to merge. `submitted` must be
   unreachable with an empty diff -- there is no such thing as "ready to merge" with
   nothing to merge. Prefer a distinct state or a hard refusal in `weave pull`.

Also: `weave pull` should REFUSE an empty branch outright, and
`weaveCloseRegisterOnMerge` must not close a register entry for a merge that carried no
change.

## Verify

    go test ./pkg/weave/
    # plus a test: a run with KilledBy set and 0 commits must never reach "submitted",
    # and `weave pull` on an empty branch must refuse and leave the issue OPEN.
