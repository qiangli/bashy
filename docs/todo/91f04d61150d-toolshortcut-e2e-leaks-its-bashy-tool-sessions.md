---
id: 91f04d61150d
kind: test
title: toolshortcut e2e leaks its bashy <tool> sessions
seq: 3252
status: assigned
priority: p2
created: 2026-11-20T11:16:44.19335Z
assignee: muse-wonder-full
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

TestToolShortcutE2E leaves the launched bashy agy session and its sh fake running after the test (orphaned, parent pid 1). 2026-11-20 the dev box held 19 such pairs from runs over 11 h; each holds a PTY and a room card. Root cause to confirm: test cleanup does not kill the session process group when quit/steer cleanup fails. Fix in the test (kill the group in t.Cleanup).
