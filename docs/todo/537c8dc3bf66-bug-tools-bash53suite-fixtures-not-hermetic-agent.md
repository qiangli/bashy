---
id: 537c8dc3bf66
kind: bug
title: 'bug: tools/bash53suite fixtures not hermetic — agent-detection banner leaks into output under a Claude session'
seq: 2937
status: todo
priority: p2
created: 2026-10-30T17:16:53.820646Z
---

Seen 2026-10-30 (S537 weave pull 348 verify gate on bashy main 7f2eaf68). TestGuardedFixtureNormalRun and TestFixtureNormalExitKillsRemainingProcessGroup fail: got 'bashy: claude detected, and this repo has no agent config …' instead of 'guarded-normal'. Fix: scrub agent env (CLAUDECODE/CLAUDE_CODE_*/BASHY_HINTS=off) in the fixture runner. Repro: GOWORK=off go test ./tools/bash53suite/ from a Claude Code shell. Blocks weave pull verify for every run on this host.
