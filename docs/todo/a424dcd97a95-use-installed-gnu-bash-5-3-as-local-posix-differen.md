---
id: a424dcd97a95
kind: test
title: Use installed GNU Bash 5.3 as local POSIX differential oracle
seq: 343
status: todo
priority: p1
labels:
    - posix
created: 2026-09-24T20:49:30.751952Z
sprint: 274
sprint_id: e14729b9-4d75-5803-aca0-594e943ca2df
sprint_title: Bashy GNU Bash 5.3, POSIX mode and full Go tests across platforms
---

Extend posix-parity.sh and austin-defects.sh with explicit local upstream GNU Bash 5.3 oracle options for native macOS/Linux differential runs, without containers. Add preload-first candidate-only Austin execution on Windows. Fix posix-parity-pty.sh terminal marker parsing when GNU readline emits repeated prompt prefixes before completion, then rerun affected PTY lanes. Fail closed on wrong oracle version/self-reference and preserve default OCI behavior. Harness-only change for Sprint 274 survey; no Bashy product fix.
