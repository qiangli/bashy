---
id: beeb7ccce6a9
kind: bug
title: bashy git ls-remote unsupported on the native door
seq: 422
status: wontfix
priority: p1
labels:
    - git
created: 2026-10-09T05:25:23.144587Z
closed: 2026-10-09T06:00:50.188114Z
---

By design, closed 2026-10-09: ls-remote is host-only in the one-door (yoke/git/external.go names ls-remote among verbs callers send to the host); bashy git --external=true ls-remote works (verified on b3c770f4) and internal callers (sdlc deploy_tag) already use yokegit.RunChecked. Reopen only if a native ls-remote is wanted.
