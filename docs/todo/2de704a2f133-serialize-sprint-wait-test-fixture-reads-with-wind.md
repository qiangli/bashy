---
id: 2de704a2f133
kind: bug
title: Serialize sprint-wait test fixture reads with Windows queue replacement
seq: 388
status: todo
priority: p0
labels:
    - windows
    - ci
created: 2026-10-02T06:35:58.096436Z
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
---

Bashy 13bbe722 Windows CI TestSprintWaitReadsStructuredData failed with a sharing violation while its test goroutine renamed queue.json against defaultSprintWaitRuntime polling. The test checks structured reader and stage detection; synchronize only this fixture writer and reader so it exercises the real reader without Windows replacement overlap. Acceptance: focused test and Windows Bashy CI pass; production behavior unchanged.

Continuity 2026-10-02: run 36973692528 Windows test failed at `sprint_wait_test.go:310` with a queue.json sharing violation during the fixture's 20 ms replacement. The previous c5d087 test matrix passed Windows; this is intermittent. The test now serializes its fixture rename and real structured reader, checks the fixture update error, and excludes ambient host disk/conductor events from the stage assertion. Focused macOS test passes 20 repetitions. Windows confirmation requires the next CI run; no production code changed.
