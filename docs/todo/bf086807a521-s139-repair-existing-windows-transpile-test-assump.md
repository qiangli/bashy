---
id: bf086807a521
kind: task
title: 'S139: repair existing Windows transpile test assumptions to unblock delivery CI'
seq: 249
status: assigned
priority: p1
created: 2026-09-08T07:35:43.729631Z
assignee: sprint139-manager
sprint: 139
---

Required delivery CI already fails on Windows at main91d386f: file-not-found checks Unix error text, standalone execute/ModuleInputDirectory omit .exe, rollback mode assumes Unix permissions. Correct platform-specific test assertions and fixture binary naming without weakening portable error/rollback guarantees or changing product semantics. Gate: targeted Darwin tests, Windows cross-compilation and actual Windows CI green. Found by Sprint139 publication review, run34174218093.
