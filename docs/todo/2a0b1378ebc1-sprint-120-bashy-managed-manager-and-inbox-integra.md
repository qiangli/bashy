---
id: 2a0b1378ebc1
kind: task
title: Sprint 120 Bashy managed-manager and inbox integration
seq: 201
status: done
priority: p0
created: 2026-09-04T02:10:49.657624Z
assignee: qiangli
sprint: 120
closed: 2026-09-04T03:45:10.053744Z
---

Own and integrate the Sprint 120 Bashy-side changes: managed sprint-manager adapter/session wiring, explicit owner CLI/help/skill contract, human mailbox directed-Meet regression, focused and cross-platform tests. Preserve Sprint 114 todo files and all unrelated changes. Gate: go test ./internal/agentos ./skills plus hermetic umbrella script/e2e-agent-inbox.sh against a binary built from this checkout.
