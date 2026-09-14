---
id: 591c6dc0b342
kind: task
title: 'S5: download e2e (httptest) + umbrella gate script + agentic-eval row'
seq: 281
status: done
priority: p1
created: 2026-09-14T19:42:32.886818Z
assignee: voussoir
sprint: 179
closed: 2026-09-14T20:13:38.54433Z
closed_by: voussoir
---

commands_registry_e2e_test.go: provision, cache hit, wrong digest refused, no digest refused. dhnt script/e2e-commands-registry.sh 19 cases incl. --posix resolution + VSC_PROFILE=cert exclusion; script/bashy-agentic-eval registered-yield row. Gate: both GREEN on the built binary.
