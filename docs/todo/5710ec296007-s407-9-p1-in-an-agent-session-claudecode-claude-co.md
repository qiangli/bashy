---
id: 5710ec296007
kind: bug
title: 'S407.9 P1: in an agent session (CLAUDECODE / CLAUDE_CODE_ENTRYPOINT set) bashy adds ~1 s to EVERY external command'
seq: 1951
status: todo
priority: p1
labels:
    - performance
    - agent-mode
created: 2026-10-25T04:51:43.026484Z
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Found 2026-10-25 while verifying S407.6/S407.7 on macOS (Dragon). Script: i=0; while [ $i -lt 5 ]; do /usr/bin/true; i=$((i+1)); done, timed per command:
- env -i HOME PATH + bashy (rc.15 candidate, init now 15 ms): ~410 ms/cmd (host spawn is slow on this Mac; /bin/bash ~200-400 ms too).
- same + CLAUDECODE=1: ~1430 ms/cmd. Same + CLAUDE_CODE_ENTRYPOINT=cli: ~1430 ms/cmd. Every other agent env var (CLAUDE_CODE_SESSION_ID, BASHY_AGENT_MANIFEST, BASHY_OUTPUT_PARENT, ...) is neutral. BASHY_HINTS=off does NOT help.
- The installed canonical bashy 59e6be1 shows the same (~1.2 s/cmd in a Claude Code session), so every agent driving bashy pays ~1 s per external command.
Agent detection table: yoke/pkg/fleet/detect.go ({"CLAUDECODE","claude"}, {"CLAUDE_CODE_ENTRYPOINT","claude"}); find what runs per external command once an agent is detected (likely a subprocess, a network/socket call, a fleet/registry lookup, an audit/activity write with fsync, or a re-exec), using a CPU/trace profile or dtruss/strace rather than guessing.
Do: red test first (a Go test or script that times N external commands with CLAUDECODE=1 vs unset and fails if the difference exceeds ~10 ms/cmd), fix at the root (cache detection once per process, move any per-command agent bookkeeping off the exec path or make it async/batched), keep agent features working. Measure on macOS and Linux. make test-bash 86/86. Sprint/Story/Story-ID trailers; list exact SHAs per repo (yoke changes go through yoke; conductor publishes and bumps pins).
