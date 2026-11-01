---
id: 79f96653e6cd
kind: bug
title: S557 restore standalone and Windows agentos test gates before release-preparation validation
seq: 3034
status: assigned
priority: p1
created: 2026-11-01T09:27:12.593175Z
weave: 1
assignee: claude-opus5.5
sprint: 557
sprint_id: f5ccf563-f5d1-5b94-8be4-9388e51ef86d
sprint_title: 'Bashy v1.0.0 preparation: split private/public development and retire qiangli/dhnt'
---

Baseline GitHub run 41281250337 at Bashy558ff064 fails all OS before migration: TestCommandDocSourceNames omits newly shipped desk; TestSrcPinsMatchLinkedModules source yoke pin mismatches linked89df473c6035. Windows also TestPodmanConfOverrideLinuxPaths, TestRepairUnitsMaterializeAsBashSharpShims, TestRepairPosixLinksNeverShadowWrapperUnits and timeout-killed repair suite/profile tests leave children holding TempDir. Reproduce targeted cases, fix actual expectations/contracts or lifecycle bug without weakening tests, maintain public-only content. No legacy workspace edits. Work only in this Bashy checkout; preserve parent integration. Short targeted tests okay; full builds/suites require remote runner. No commit/push until manager review. Full evidence in GitHub failing job logs.
