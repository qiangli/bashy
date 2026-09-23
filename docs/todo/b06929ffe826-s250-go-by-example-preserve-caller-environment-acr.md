---
id: b06929ffe826
kind: bug
title: S250 Go by Example preserve caller environment across Bashy startup
seq: 324
status: todo
priority: p0
created: 2026-09-23T11:00:53.091809Z
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

Linux Story #89 full Go by Example gate on candidate sh 778ca879/Bashy f72114a: 255 attempts, only environment-variables.go and mutexes.go interpreted red. Environment row 20 differs only by extra BASHY_AGENT_MANIFEST in interpreted output; oracle and compiled pass. cli.gosource_environment.go captures caller os.Environ before agentos.init, but cli.Main unconditionally refreshes snapshot after init (commit 7fc8a185), leaking the manifest. Restore original caller environment for ordinary Go-source CLI while preserving owned child-frame restored environment explicitly. Keep original source, gate environment, 20s limit and raw output comparison. Focused real executable differential test must cover absent and explicit manifest, then manager authenticated Go by Example replay. Parent Sprint 250 Story #89 (235209310249).
