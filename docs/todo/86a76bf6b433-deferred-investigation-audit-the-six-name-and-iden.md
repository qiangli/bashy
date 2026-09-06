---
id: 86a76bf6b433
kind: task
title: 'DEFERRED INVESTIGATION: audit the six name and identity resolvers in pkg/weave'
seq: 232
status: blocked
priority: p3
created: 2026-09-06T01:03:37.2693Z
---

A SHAPE, not a known bug. ff01bbb3 fixed `checkpoint` filing the continuity brief under
the literal "conductor" — the cause was TWO functions answering "who is writing this"
(weaveConductorName vs weaveStoryConductorName) with the wrong one wired in. The bug is
fixed; the shape that produced it is not.

DEFERRED BY OPERATOR FOR SPRINT 127: this is preventative investigation, not a confirmed
bug or a test for a specified behavior. Do not staff it in the tests-and-bug-fixes-only
lane. Reopen it in a later sprint if the audit is explicitly authorized.

MEASURED: pkg/weave has SIX name/identity resolvers —
  weaveAgentName, weaveCodingIdentity, weaveConductorIdentity, weaveConductorName,
  weaveStoryConductorName, weaveWorkspaceOwner

TASK: for each, state in one line WHICH QUESTION IT ANSWERS and WHO SHOULD CALL IT, then
find call sites answering that question with the wrong one. The checkpoint defect was
invisible for as long as it was because the command REPORTED the right name to the operator
while RECORDING the wrong one — so read what each caller PERSISTS, not what it prints.

Known-correct and not to be "unified": claim/yield/submit use the ephemeral resolver
deliberately (a worker claim must not be attributed to the manager), and abort likewise (an
operator aborting a stuck manager is not that manager). Any consolidation must preserve
both.

DELIVERABLE: the table, plus a filed story per miswiring found. If none is found, that is a
result — record it and close.
