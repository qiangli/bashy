---
id: 3d864ad1dccc
kind: bug
title: weave start --resume blocked by retained reservation with no sanctioned reconcile verb
seq: 372
status: todo
priority: p2
created: 2026-09-30T19:18:12.812055Z
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Conductor 2026-09-30 (sprint 342): run bashy#9 failed as wrapper-died; reaper cleared WrapperPid but left ResourceReservationID set with ResourceTerminated=false. Every resume path refuses: 'prior child termination is unverified; reconcile its retained reservation before restart' (yoke/pkg/weave/weave_impl.go:3359). Child verified dead via ps; doctor/finalize/kill/salvage offer no flag that flips the bit, and kill requires working state. Workaround used: committed partial work as ce1bdf9, abandon --force (salvage ref), fresh run. Needed: a sanctioned reconcile verb (e.g. weave reconcile <id> that verifies child death by PID and marks terminated) or make the reaper set it when no child exists.
