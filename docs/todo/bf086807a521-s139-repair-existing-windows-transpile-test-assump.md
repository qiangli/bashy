---
id: bf086807a521
kind: task
title: 'S139: repair existing Windows transpile test assumptions to unblock delivery CI'
seq: 249
status: done
priority: p1
created: 2026-09-08T07:35:43.729631Z
assignee: sprint139-manager
sprint: 139
closed: 2026-09-08T09:16:00.916901Z
---

Required delivery CI already fails on Windows at main91d386f: file-not-found checks Unix error text, standalone execute/ModuleInputDirectory omit .exe, rollback mode assumes Unix permissions. Correct platform-specific test assertions and fixture binary naming without weakening portable error/rollback guarantees or changing product semantics. Gate: targeted Darwin tests, Windows cross-compilation and actual Windows CI green. Found by Sprint139 publication review, run34174218093.

Delivered 2026-09-08 through PR #11: https://github.com/qiangli/bashy/pull/11 . The published P0 head `04aff62` includes the equivalent Windows repair `5fa3f3d` / `77af18c`. Native Windows build/vet/tests, e2e dispatch and six-platform cross-build passed in https://github.com/qiangli/bashy/actions/runs/34206384861 ; Linux, macOS and Bash53 jobs also passed. The P0 owner installed and verified that exact revision.
