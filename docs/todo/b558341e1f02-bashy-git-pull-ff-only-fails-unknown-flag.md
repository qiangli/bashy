---
id: b558341e1f02
kind: bug
title: 'bashy git pull --ff-only fails: unknown flag'
seq: 421
status: todo
priority: p1
labels:
    - git
created: 2026-10-09T05:25:21.72795Z
---

`bashy git pull --ff-only origin main` fails with "unknown flag: --ff-only" in the dedicated cobra pull verb, though native pull is fast-forward-only by design; the flag should be accepted (it is the default behavior).  Found by the Sprint 404 smoke (story 2324bae4d6fa) of bashy git through the native one-door; fails identically with and without -C, so it is independent of the -C fix.  (found by the Sprint 404 smoke, 2026-10-09; native engine only, works with --external)
