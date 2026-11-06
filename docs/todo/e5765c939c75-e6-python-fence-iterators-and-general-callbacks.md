---
id: e5765c939c75
kind: feature
title: E6 Python fence iterators and general callbacks
seq: 3062
status: assigned
priority: p1
labels:
    - clean-baseline
    - fence-corpora
created: 2026-11-04T13:54:22.788224Z
assignee: codex-gpt5.6-sol-w45
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Split from B3 for independent ownership (operator decision 2026-11-03: implement both in S562). Design input: sh docs/s562/go-internal-python-handles.md (iterators and callbacks sections). (1) Iterators: adapt an authenticated Python handle into the existing polyglot Iterator lifecycle with a small worker operation; decide EOF shape and early-break cleanup first; interpreted + lowered. (2) Callbacks: design note first (call-scoped vs retained, nested calls, exceptions, cancellation, callback-returned handles), manager review, then implementation reusing host callback tables; interpreted + lowered. Acceptance: red/green regressions for each; go test ./polyglot/... and the -tags full interp/lower fence selections green except baseline-identical failures; tutorial examples that use them become cases.

Execution plan: docs/plan-s562-master-execution.md. Delivery commits carry Sprint, Story and Story-ID trailers.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
