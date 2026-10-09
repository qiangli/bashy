---
id: e27d4c067f49
kind: bug
title: native bashy git diff/show/pull subcommands reject engine flags (--stat, --format, --ff-only)
seq: 420
status: todo
priority: p1
labels:
    - git
created: 2026-10-09T05:25:20.298812Z
---

Verified 2026-10-09 on bashy b3c770f4 (scratch repo + bare remote): bashy git diff --stat, show --stat, show --format=%s -s and pull --ff-only fail natively with "unknown flag" from bashy's own cobra subcommands (internal/agentos/git.go gitDiffCmd/gitShowCmd/gitPullCmd); all work with --external=true. Engine context (yoke/git): nativeDiff parses --stat; typed Pull is already fast-forward-only but nativePull rejects any flag. Fix as was done for log in 7caeed0c: let flags the subcommand does not own reach the engine (accept --ff-only as the native pull's own semantics), red test per flag; anything the engine cannot serve returns the --external hint. Merges former todo b558341e1f02 (pull --ff-only).
