---
id: 9e8310f8263f
kind: bug
title: A3d Fix the 5 Windows-only internal/agentos test failures
seq: 3068
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T17:50:57.493602Z
weave: 17
assignee: claude-sonnet5.5
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T20:13:18.224996Z
closed_by: claude-opus5.5
---

Operator: fix now, no follow-ups. CI run 41794723828 (candidate f1bcd711) windows-latest fails 5 tests that also failed on baseline d34c29b6: (1) TestPodmanConfOverrideLinuxPaths; (2) TestRepairUnitsMaterializeAsBashSharpShims — repairprofile_test.go:282 'go shim must run the pinned bashy's go'; (3) TestRepairPosixLinksNeverShadowWrapperUnits — repairprofile_test.go:845 'wrapper sleep must stay the authoritative Bash# unit'; (4) TestRepairSuiteTimeoutKilledNeverPasses and (5) TestRepairProfileKilledSuiteBlocksPassAndCompletion — testing.go TempDir RemoveAll 'file being used by another process' after a killed suite (likely an orphan child still holding the dir: the kill must terminate the whole process tree / Job Object, then wait, before returning). Read the logs: gh run view 41794723828 -R qiangli/bashy --log-failed. Fix the product or test at the owning layer; NO skips, no weakened assertions, no timeout growth. Verify on the Windows test host (ask the manager for host access; the claim is coordinated with S562) with GOWORK=off go test ./internal/agentos -run '<the 5>' -count=3, plus macOS/Linux go test ./internal/agentos stays green. These packages are outside both cert builds: sh scripts/cert-identity-check.sh docs/s561/candidate-manifest.json HEAD must print SAME for bash and bashy — record it. Deliver docs/s561/a3d-results.md. Trailers Sprint: #561, Story: <seq>, Story-ID: <id>; BASHY_AGENT = you; do not push.
