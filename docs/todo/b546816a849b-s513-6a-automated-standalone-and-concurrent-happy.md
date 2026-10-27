---
id: b546816a849b
kind: test
title: S513.6a automated standalone and concurrent happy-path browser E2E for Matrix, whiteboard and brainstorm
seq: 2746
status: todo
priority: p0
created: 2026-10-27T18:11:07.81892Z
sprint: 513
sprint_id: 8bb3bd0b-2aa9-5952-92ca-6dda0194fca6
sprint_title: 'Bashy collaboration: Matrix chat + whiteboard/brainstorm for everyone; lern live UAT'
---

Implement umbrella story 5100f0603023 in Bashy source. Build reproducible automated E2E entry point that invokes each independent tool runner: Matrix real signed tuwunel two-user login/DM/text/image; independent whiteboard real browser create/draw/notes/sync/restart/PNG+JSON; independent brainstorm real browser create/private-write/reveal/vote/close/export. Run all three simultaneously with distinct ports/data, stop each one in turn and assert the other two still work. Use actual signed service and real browser, no mock-only PASS, no downloads; absent prerequisites fail closed/BLOCKED. Current Bashy main has whiteboard and brainstorm, Matrix #298 commit 31d39635 awaits #303 safety repair, Cloudbox #301 awaits #304. Avoid editing provision.py/cloudbox_shim.py or other worker-owned paths; write harness in new test-focused path and coordinate integration before live gate. S514 R11 holds novidesign until DONE; local lightweight code/unit work now, manager grants host use later. Commit normal Sprint513 Story trailers, sprint submit under worker identity, report exact gate and limitation. Manager gates/merges after Matrix repair and Cloudbox shim are integrated, keeps real lern UAT separate.
