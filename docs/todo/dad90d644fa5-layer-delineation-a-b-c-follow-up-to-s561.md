---
id: dad90d644fa5
kind: refactor
title: Layer delineation A/B/C (follow-up to S561)
seq: 3067
status: todo
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T14:14:05.134297Z
---

Plan: docs/plan-layer-delineation.md. Enforce A Classic (bash+coreutils, POSIX-certified) / B Bash# + fenced runtimes / C Yoke (genie, shaping, optional coreutils) in the import graph so A's certification evidence is never voided by B/C work. Stories L1 sh Bash# engine out of interp (V2), L2 cli Bash# front via injection hook (V1), L3 cert base drops yoke/pkg/fleet + bashsharp/transpile (V3/V4), L4 posixproviders to yoke, L5 per-layer ratchets + A5 evidence identity. Deferred until the S561 licensed run on f1bcd711 completes. Operator decision needed: remove Bash# (--bashsharp/--bashpp) from the Classic bash binary. Coordinate with S562 (B) and S563 (C) managers.
