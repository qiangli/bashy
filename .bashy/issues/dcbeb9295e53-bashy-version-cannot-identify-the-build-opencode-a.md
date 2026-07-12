---
id: dcbeb9295e53
kind: feature
title: 'bashy --version cannot identify the build (opencode attempt — deliberate A/B against run #13)'
status: triaged
stage: code
priority: p2
labels:
    - bakeoff
reporter: qiangli
created: 2026-07-12T23:05:05.484612Z
weave: 14
---

## This is a DELIBERATE PARALLEL ATTEMPT

The same task is being worked by codex on run #13 (register 352edbea). This is a
bake-off to measure whether opencode can handle it. Exactly ONE of the two branches
will merge; the loser is abandoned. That is the intent, not waste.

Do not coordinate with the other run. Solve it independently.

## Problem

    $ bashy --version
    GNU bash, version 5.3.0(1)-bashy-dev

That string is identical for every dev build ever made. A user asked "have you built
and installed the new bashy?" and the tool could not answer -- you had to stat the
binary and compare mtimes. A shell that cannot identify its own build is a shell you
cannot trust a bug report against.

## Task

Stamp the short git SHA (and a dirty marker) into the version, via the ldflags the
Makefile already uses for `internal/cli.bashVersion`:

    GNU bash, version 5.3.0(1)-bashy-dev (6e1d934)
    GNU bash, version 5.3.0(1)-bashy-dev (6e1d934-dirty)

Constraints:
  - GNU bash prints `GNU bash, version <v> (<machine>)`. The bash-5.3 conformance
    fixtures PARSE this string. APPEND; do not restructure. If you break the shape,
    86/86 becomes 85/86 and the release gate fails.
  - A tagged release build should show the tag, not a raw SHA.
  - Degrade gracefully: a source tarball with no .git must still BUILD and print
    something honest (no suffix), never fail.
  - `bin/bash` (the pure drop-in) and `bin/bashy` must agree.
  - Surface it in `bashy context --json` as well (runtime.build) -- that is the FIRST
    call an agent makes, and "which build am I on" belongs there.

## Verify (this is the gate; it decides, not your own report)

    make build && bin/bashy --version | grep -E '\([0-9a-f]{7,}(-dirty)?\)'
    make build && bin/bash  --version | grep -E '\([0-9a-f]{7,}(-dirty)?\)'
    make test-bash-parallel        # MUST still be 86 passed, 0 failed, 0 skipped
    go test ./internal/...
