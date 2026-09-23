---
id: 2f0ec87e1db6
kind: bug
title: Restore Windows bash-5.3 invocation binary-file diagnostic with POSIX PATH
seq: 316
status: done
priority: p0
labels:
    - windows
    - pcts
created: 2026-09-23T02:47:27.601033Z
assignee: s253-invocation-fix
sprint: 253
sprint_id: d25e93b0-ad03-56f2-831f-1e9f626e609c
sprint_title: 'Windows fixtures 76/86 to done: one regression, four found causes, one probe, one provisioning'
closed: 2026-09-23T03:00:19.702628Z
closed_by: sprint253-manager
---

Combined production bash-5.3 conformance run 35810674438 reports Windows invocation fixture 85/86: line 104 expects cannot execute binary file but Bashy prints No such file or directory after PATH=/bin:/usr/bin and ${THIS_SH} ls. Earlier run 35806025977 passed invocation. Diagnose exact regression, restore Bash-compatible operand search and binary diagnostic on Windows, verify focused fixture without full corpus. Maintain Bashy-owned command semantics and host-derived resource values.
