---
id: de8e2cc0d87b
kind: bug
title: yoke/dag adapter lost the sprint capacity services that runDagDispatch mounted in-process (P4 parity gap)
seq: 3303
status: done
priority: p1
labels:
    - bashy
    - yoke
    - adapter
created: 2026-11-22T14:32:53.82794Z
assignee: claude-fable5.1
sprint: 692
closed: 2026-11-22T14:41:35.523665Z
closed_by: claude-fable5.1
---

Found by P5 aa693f4a while proving deadness (bashy 2d7f52ad left the old code as reference). Before the adapter conversion, bashy dag ran through internal/agentos runDagDispatch, which mounted the sprint capacity services (sprint-aware scheduling) around the dag engine. yoke/dag/main.go does not mount them, so 'bashy dag' on main no longer honours sprint capacity. Fix: port the mount into the yoke/dag adapter (duplicate the glue into yoke/dag per the independence rule; shared logic may live in a yoke library package); then remove the dead in-process copy in bashy. Unit test in yoke/dag asserting the services are mounted (or a bashy regression test that was green at rc.37). Root cause before fixing: confirm by reading runDagDispatch vs yoke/dag/main.go and record the exact missing calls on this story. TEST RULE (operator 2026-11-22): build + smallest targeted unit tests only, then submit; the one full run is the rc.40 candidate gate 458628c0. Start on current origin main; Sprint/Story/Story-ID + Agent trailers; bashy commit+push first, then the conductor bumps the umbrella pin. Owner codex, one run with the sibling stories filed at the same time (claim each).

Root cause confirmed at 25b60ada: runDagDispatch calls dag.AddCapacityCommands(cmd, sprintCapacityServices()), but yoke/dag passes dag.CapacityServices{}. Exact missing callbacks: resources.LookupProcessIdentity; resources.ObserveHost plus destination disk policy selection; collectSprintMonitor JSON observation. Restored in adapter with standalone read-only monitor closure; legacy dead copies moved to test-only helpers. Focused mounted capacity receive, process identity, disk probe and standalone-dependency tests PASS; dag and cmd/bashy builds PASS. Full gate deferred to 458628c0.
