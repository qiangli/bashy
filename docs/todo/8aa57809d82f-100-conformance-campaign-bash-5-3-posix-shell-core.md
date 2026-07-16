---
id: 8aa57809d82f
kind: task
title: 100% conformance campaign — bash 5.3 + POSIX shell + coreutils, absolute 100% pass, no excuses
seq: 1
status: todo
priority: p1
created: 2026-07-16T09:54:40.999643Z
---

Plan of record: docs/conformance-100-campaign.md.
(a) GOAL: GNU bash 5.3-compatible POSIX shell + POSIX-compliant coreutils, 100% pass rate. "bash/GNU also fails" is NOT an accepted excuse — bashy must pass anyway (superset ceiling). bash-5.3 fixtures 100%; shell at bash-parity; COREUTILS is the active front (pkg/bre regex depth → NO-list argv-runner → long tail).
(b) ACCELERATOR: auto-fixer + distributed test-running across all registered agents (chunk suites, dag --fleet fan-out, route failures to fixers, gate on our differential, loop until dry).
(c) ECOSYSTEM: any ycode/outpost/cloudbox change that speeds up (a) — do it even if it pauses (a); make Tessaro better for bashy.
ROLES: claude=steward (monitor all conductors, prevent agent overload, rebalance codex→less-loaded equally-capable agents); codex=conductor driving (a); claude delegates SELF to (b); ycode drives (c) reactively.
DISCIPLINE: never shell out; PCTS numbers not published (world-readable repos); gate is evidence not a status label.
