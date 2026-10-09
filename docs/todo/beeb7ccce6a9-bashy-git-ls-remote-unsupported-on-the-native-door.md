---
id: beeb7ccce6a9
kind: bug
title: bashy git ls-remote unsupported on the native door
seq: 422
status: todo
priority: p1
labels:
    - git
created: 2026-10-09T05:25:23.144587Z
---

`bashy git ls-remote origin` fails with 'unknown git subcommand "ls-remote"' and has no unimplemented-verb hint or native handler; agents use it routinely. Add a native ls-remote (go-git remote List) or at least an --external hint entry.  Found by the Sprint 404 smoke (story 2324bae4d6fa) of bashy git through the native one-door; fails identically with and without -C, so it is independent of the -C fix.  (found by the Sprint 404 smoke, 2026-10-09; native engine only, works with --external)
