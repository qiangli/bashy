---
id: 66d983109f50
kind: bug
title: 4 internal/agentos tests fail on clean main (pre-existing; block weave suite gate)
seq: 3249
status: todo
priority: p2
labels:
    - tests
created: 2026-11-19T23:38:17.766634Z
sprint: 676
sprint_id: cc17f40d-d5ff-5d27-bbed-b8563759cde6
sprint_title: 'Fleet hardening after the tool-shortcut tests: steerable ycode/zcode/dsh + pre-existing test failures'
---

Found 2026-11-19 when the weave suite gate refused run #113 (merged by hand at 73c7d24c with evidence). The same 4 fail on clean main 866ba761 on the dev host (GOWORK=off go test ./internal/agentos/ -run ...): TestCommonOptionsVocabularyReport (5.5s), TestServiceMonitorSeedInternalAndCustomShareOneMechanism (10.3s), TestMonitorForwardsTermToARealProcessGroup (20.0s), TestMonitorKillsARealChildThatIgnoresTerm (20.5s). The Monitor* pair time out at ~20s (process-group/signal behaviour on macOS?). Per the root-cause rule: name the cause per test (venue vs product vs test defect) before any fix; then fix where the cause lives. They block every weave pull with a suite gate in this repo.
