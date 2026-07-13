---
id: 352edbeab4eb
kind: feature
title: bashy --version cannot tell you WHICH build you are running
status: closed
stage: code
priority: p2
reporter: qiangli
created: 2026-07-12T22:58:18.388268Z
closed: 2026-07-12T23:46:03.910859Z
resolution: fixed
closed_by: codex-m
---

## Motivation (from real use, today)

    $ bashy --version
    GNU bash, version 5.3.0(1)-bashy-dev

That string is identical for every dev build ever made. During a live dogfood the
question "have you built and installed the new bashy?" could not be answered by the
tool itself — you had to stat the binary and compare mtimes. A shell that cannot
identify its own build is a shell you cannot trust a bug report against.

## Task

Stamp the short git SHA (and dirty flag) into the version, via the ldflags the
Makefile already uses for `internal/cli.bashVersion`:

    GNU bash, version 5.3.0(1)-bashy-dev (6e1d934)
    GNU bash, version 5.3.0(1)-bashy-dev (6e1d934-dirty)

Constraints:
  - GNU bash prints `GNU bash, version <v> (<machine>)` — do NOT break the shape that
    scripts and the conformance fixtures parse. Append, do not restructure.
  - A release build (a tag) should show the tag, not a raw SHA.
  - It must degrade gracefully: a source tarball with no .git must still build and
    print something honest (e.g. no suffix), never fail the build.
  - `bin/bash` (the pure drop-in) and `bin/bashy` should agree.
  - Surface it in `bashy context --json` too (runtime.build) — that is the FIRST call
    an agent makes, and it is exactly where "which build am I on" belongs.

## Verify

    make build && bin/bashy --version | grep -E '\([0-9a-f]{7,}(-dirty)?\)'
    make test-bash-parallel   # 86/86 — the fixtures parse the version string
