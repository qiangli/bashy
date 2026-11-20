---
id: 39669b0dd32d
kind: bug
title: 'bashy git applet: checkout/push/pull lack -q/--quiet (cobra front door flag gaps)'
seq: 3268
status: done
priority: p3
created: 2026-11-20T21:24:30.75529Z
assignee: muse-wonder-full
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-20T21:49:25.755694Z
closed_by: claude-opus5.5
---

Spun off from 4c60ebe8 (S576) per conductor. bashy/internal/agentos/git.go: gitCheckoutCmd/gitPushCmd/gitPullCmd are cobra-typed commands with narrow flag sets; real-git -q forms (checkout -q, push -q, pull -q) die in pflag like commit -q did. No surveyed worker usage yet (weave scripts surveyed 2026-11-20); fix the same way as commit (add -q/--quiet, wire output suppression) when picked up. See kb bashy-git-is-two-layers-cobra-subcommands-shadow-the-yoke-engine.
