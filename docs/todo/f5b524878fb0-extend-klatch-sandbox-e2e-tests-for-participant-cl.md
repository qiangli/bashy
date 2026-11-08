---
id: f5b524878fb0
kind: test
title: Extend Klatch sandbox e2e tests for participant CLI
seq: 2829
status: done
priority: p1
created: 2026-11-08T17:43:17.134328Z
weave: 55
assignee: claude-opus5.5
sprint: 597
sprint_id: ce9248d4-2579-5c97-857e-de17d182395f
sprint_title: Klatch participant CLI MVP
closed: 2026-11-08T19:01:14.17317Z
closed_by: codex-gpt6-sol-klatch
---

Extend existing public bashy/tests/klatch-e2e/run.py and test_run.py, reusing sandbox lifecycle. Exercise PRIMARY Bash# participant command surface and small Node helper as appropriate; cover sealed messaging and attachments, auth/errors, JSON/secret handling, contacts/conversations/messages, watch reconnect and provisioning regression. Preserve topology PASS/FAIL/BLOCKED semantics; no private satellite harness. Acceptance: runnable sandbox e2e tests and measured results, BLOCKED distinct from PASS. ALL builds/tests on the claimed shared test host through the established outpost SSH route; development host editing/coordination only. Coordinate contract with client and Bash# owners.
