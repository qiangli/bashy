---
id: 3caea732b6a5
kind: task
title: S531.0c manager integration fixes found in live checks (bashy)
seq: 2843
status: done
priority: p1
created: 2026-10-29T23:49:31.698701Z
assignee: s531-manager
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T23:55:23.143225Z
closed_by: s531-manager
---

Manager-owned story for small live-found fixes s531-manager authors itself in bashy. First: wire the leading '-c k=v' forward in the git front door (0f5576ae defined hasGitConfigOverride/gitRunEngineArgv but never called them; live 'bashy git -c ... commit' still hit cobra).
