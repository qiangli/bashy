---
id: 96b4bac59b0a
kind: test
title: 4 agentos tests fail on main since before 2026-11-20 (TurnAdmission x2, SkillHelp goldens x2)
seq: 3254
status: done
priority: p2
created: 2026-11-20T12:29:27.697653Z
assignee: claude-opus5.5-s676
sprint: 676
sprint_id: cc17f40d-d5ff-5d27-bbed-b8563759cde6
sprint_title: 'Fleet hardening after the tool-shortcut tests: steerable ycode/zcode/dsh + pre-existing test failures'
closed: 2026-11-20T16:01:36.219219Z
closed_by: claude-opus5.5-s676
---

Failing at 845699ca and at 71649157 (same names): TestUnifiedTurnAdmissionAcknowledgesOnlyRenderedRoleAndBusRecords, TestUnifiedTurnAdmissionAcknowledgesMeetCopyWithItsMBOrigin, TestSkillHelpClassicSnapshots, TestSkillHelpGoldens. Pre-existing, not caused by the 11-20 merges. Root cause not yet recorded. Also: TestToolShortcutE2E/synergy tests use bin/bashy when present, so a stale bin/bashy runs old code (rebuild before go test).

Root cause (2026-11-20, conductor): TEST defects from intentional yoke changes. TurnAdmission x2: yoke f4a1c2f2 per-turn cap (1 item + counter) — proven by reverting only the cap in current yoke; tests lift the cap (BASHY_CHAT_INBOX_MAX=100) and fix a ":1" vs ":10" id match. SkillHelp x2: sprint help gained await/directive/reject (S576 merges); goldens regenerated. Fixed in bashy 084a7f30; full internal/agentos green.
