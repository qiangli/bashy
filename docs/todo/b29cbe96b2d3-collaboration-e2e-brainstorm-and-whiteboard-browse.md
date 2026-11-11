---
id: b29cbe96b2d3
kind: bug
title: 'collaboration-e2e: brainstorm and whiteboard browser runners fail on Linux with Playwright 1.65 (pre-existing, before Sprint 614)'
seq: 3222
status: todo
priority: p2
labels:
    - e2e
    - collaboration
created: 2026-11-11T09:31:46.300152Z
---

Found by Sprint 614 S6 (umbrella story 6ace118d16cb) on the Linux test host (WSL Ubuntu 24.04, node 22.11.0, Playwright 1.65.0, Chromium build 1254, tuwunel 1.9.4-bashy.2 installed beforehand).

Pre-existing: tests/collaboration-e2e/run.py fails, and both browser runners fail when run alone, identically at bashy 10d55bf3 (before Sprint 614) and at b7d70e4c:
- examples/brainstorm/browser-test.mjs: TimeoutError clicking #createSession (waiting for element to be visible, enabled and stable), browser-test.mjs:42.
- examples/whiteboard/browser-test.mjs: rc=1, failure point varies run to run between browser-test.mjs:53 (#connection 'Connected') and :66 (#connection 'Private board') for both versions (3 runs each) - flaky.
- run.py then reports 'matrix exited before concurrency barrier'.
Sprint 614 changed only the CLI files of these apps (cli.go), which the browser runners do not exercise.

Not root-caused (outside Sprint 614). Next steps: find the last green venue and Playwright/Chromium versions for this gate (docs/plan-collaboration-e2e.md), reproduce there, then bisect venue vs code. Also note in the gate preflight that Playwright needs node >= 20.
