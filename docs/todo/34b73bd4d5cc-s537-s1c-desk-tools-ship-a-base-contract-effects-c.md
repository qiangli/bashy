---
id: 34b73bd4d5cc
kind: bug
title: S537.S1c desk tools ship a base contract (@effects cap) so shaped fork/activate/undo passes the gate
seq: 2968
status: done
priority: p1
labels:
    - desk
created: 2026-10-30T20:43:46.483209Z
weave: 383
assignee: claude-opus5.5
sprint: 537
sprint_id: 3ec79210-039c-57d7-a0b5-7abfeeefbb1c
sprint_title: 'bashy desk: agent-human tool suite (notepad, form, chart, whiteboard, brainstorm) on one contract + UI kit'
closed: 2026-10-30T21:20:40.811542Z
closed_by: s537-manager
---

Bug found by the W5 shaped fork proof (manager, 2026-10-30) on bashy e070246d: desk tools ship shaped.json (S1b 85cfcad1) but NO base contract, so the shaped activation gate can never pass. Repro (isolated BASHY_HOME, candidate bashy built from main):
  bashy app add notepad --type shaped --source $PWD
  (commit a README change on branch alice-fork in $BASHY_HOME/src/notepad)
  bashy src pr open notepad alice-fork --title x ; bashy src pr verify 1
  -> verdict: fail ... reason: the base contract declares no effect cap; an uncapped run proves nothing
  bashy src activate notepad <rev> --principal device:alice -> refused (evidence verdict not pass)
Fix (every desk tool: notepad, form, chart, viewer, whiteboard, brainstorm): ship the base contract the verifier reads (see yoke pkg/webconsole/testdata/shaped/serverapp/app/contract.bsh: `@effects("read")`; pkg/shape/verify + pkg/shape/contain for what is required) with the narrowest effect cap under which the tool's own verification (build + go test) passes — e.g. read,write,exec; no net unless proven needed. Keep it in bashy's conventions (Bash# @effects decorators), no new concepts.
Red/green: examples/desk/shaped_test.go (or a new test) drives the real flow per tool with an isolated BASHY_HOME: app add --type shaped -> commit a trivial change -> src pr open -> src pr verify => pass -> src activate --principal device:alice => active -> src undo --principal device:alice => previous revision active (the fork + rollback proof). Red before the contract (current main), green after.
Gate: go test ./examples/desk/ and the desk modules you touch; root go build ./...

=== REWORK (manager) ===
Prior work preserved at refs/salvage/abandoned-380 (2b12720e: contract.bsh @effects("read,write,exec,net") + checks/go.bsh per tool + examples/desk/shaped_fork_test.go with HOME/XDG isolation). Start: `git cherry-pick 2b12720e` onto origin/main (W5b launcher rework 36b1e125 is on main now).
Linux proof on elephant (WSL2 x86_64, go1.27.2) of the candidate (pushed as branch s537/s1c-candidate, aea23bf9): TestDeskToolsForkAndRollback 5/6 PASS (form, brainstorm, viewer, chart, notepad); whiteboard FAIL: `fail checks/go.bsh` -> examples/whiteboard/server `TestWriteFailureIsNotAcknowledged` fails ("failed save changed state: 200 rev 3"), deterministic, also with plain `go test` in examples/whiteboard/server. Cause: the test forces the write failure with os.Chmod(boardsDir, 0500), which is not enforced when the user can override permissions (root / WSL). macOS passes.
Fix: make the write failure deterministic on every OS via a seam in the whiteboard server (e.g. an injectable write func on app used by save), not file permissions; check brainstorm for the same pattern. Keep the test's assertions (500, no state change, same revision).
Gate: go test in examples/whiteboard/server (and brainstorm if touched); go test -short ./examples/desk/; the manager reruns the full fork test on elephant.
