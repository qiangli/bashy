---
id: 96b4bac59b0a
kind: test
title: 4 agentos tests fail on main since before 2026-11-20 (TurnAdmission x2, SkillHelp goldens x2)
seq: 3254
status: todo
priority: p2
created: 2026-11-20T12:29:27.697653Z
sprint: 676
sprint_id: cc17f40d-d5ff-5d27-bbed-b8563759cde6
sprint_title: 'Fleet hardening after the tool-shortcut tests: steerable ycode/zcode/dsh + pre-existing test failures'
---

Failing at 845699ca and at 71649157 (same names): TestUnifiedTurnAdmissionAcknowledgesOnlyRenderedRoleAndBusRecords, TestUnifiedTurnAdmissionAcknowledgesMeetCopyWithItsMBOrigin, TestSkillHelpClassicSnapshots, TestSkillHelpGoldens. Pre-existing, not caused by the 11-20 merges. Root cause not yet recorded. Also: TestToolShortcutE2E/synergy tests use bin/bashy when present, so a stale bin/bashy runs old code (rebuild before go test).
