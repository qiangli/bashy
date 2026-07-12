---
id: 8cd873fda95a
kind: bug
title: 'agy (Antigravity CLI) is not headless: it blocks on an interactive trust prompt and idles until killed'
status: open
priority: p1
refs:
    - ../coreutils
reporter: qiangli
created: 2026-07-12T23:12:15.585032Z
---

## Observed

`weave start -- agy` launched, and agy sat on:

    Do you trust the contents of this project?
    Antigravity CLI requires permission to read, edit, and execute files here.
    > Yes, I trust this folder

waiting for a keypress. It never began work; the idle watchdog killed it after 12m.
Model line shown: "Gemini 3.5 Flash (Medium)".

## Root cause

agy's launch spec in the fleet registry (`coreutils/pkg/fleet/baseline/tools/`) has no
non-interactive / trust-bypass flag -- the analog of claude's
`--dangerously-skip-permissions` or codex's `--sandbox`. Every headless launch of agy
will hang the same way.

## The related gap

`weave fleet` reported agy as "available". Availability is currently PATH-EXISTENCE, not
"can this tool actually run unattended". A tool that hangs on a trust prompt is not
available to a fleet -- it is a 12-minute hole. The fleet check should probe headless
capability, or at minimum flag tools whose launch spec lacks a non-interactive path.

## Task

1. Find agy's non-interactive flag (or its pre-trust config file) and put it in the
   launch spec.
2. Add it to the `bashy-agent-orchestration-prior-art` per-CLI headless-flag table.
3. Make `weave fleet` distinguish "on PATH" from "launches unattended".

## Verify

    bashy weave start --run <n> -- agy   # begins work within 60s, no prompt
