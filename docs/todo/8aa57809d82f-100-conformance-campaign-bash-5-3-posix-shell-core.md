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
ROLES: steward owns conductor count/boundaries, shared integration, and the human channel; each conductor exclusively sizes and manages its own worker pool; steward may separately own or delegate disjoint direct work. codex=conductor driving (a); (b)=steward-owned claude self-fork, with no implicit merge authority over (a); (c)=separate ecosystem conductor when activated.
DISCIPLINE: never shell out; PCTS numbers not published (world-readable repos); gate is evidence not a status label.
