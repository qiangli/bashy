---
id: 2ca409cac607
kind: bug
title: 'internal/agentos: five tests red at bashy main 3c08bdad on the macOS dev box'
seq: 3247
status: todo
priority: p2
labels:
    - test
created: 2026-11-19T18:16:51.485114Z
---

Found during Sprint 671 targeted tests (2026-11-19), not caused by it. Fresh run at unchanged bashy main 3c08bdad (detached worktree, GOWORK=off): TestCommonOptionsVocabularyReport (new short options 'kb workflow -f --file' and 'meet brief -n --limit' missing from testdata/common_options_short_baseline.txt — an unreviewed CLI addition), TestServiceMonitorSeedInternalAndCustomShareOneMechanism, TestMonitorForwardsTermToARealProcessGroup, TestMonitorKillsARealChildThatIgnoresTerm (supervisord child never printed its marker). Fifth, TestGenieRecipeGeneratesCallerWorkspaceAndModel: skips without a sibling ycode; in the umbrella it fails because the sibling ycode main e5be313 does not build with GOWORK=off (missing go.sum entries for github.com/qiangli/bash and bashsharp) — venue/sibling state, cause recorded. Root cause of the other four NOT yet identified: compare with last green run, check venue, bisect before any change.
