---
id: 5d9301d113ca
kind: bug
title: S562 C fence atexit output and worker completion lifecycle
seq: 3088
status: assigned
priority: p0
created: 2026-11-05T04:17:48.147178Z
assignee: codex-gpt6-astra
sprint: 578
sprint_id: 70ba48be-af89-5ab1-8a8d-42a15fc1ddbe
sprint_title: 'S562 follow-up: remaining fence corpus work'
---

Actual C197 cached baseline192PASS: c-runtime-library/atexit-L53 prints first line then invalid worker result payload; native expects47 bytes with four callbacks reversed. Generated example_main invokes renamed orig_main; inspect C worker process completion/capture serialization vs process atexit. Diagnose exact native/process contract first, bounded repair owning polyglot/c; preserve ABI, command status, output/error and existing tests. No source/oracle rewriting or callback-output dropping. Stage real native/interp/compiled redgreen remotely; no full dev suite.

Carried from S562 (operator-approved early close 2026-11-06); deferral is not PASS. Run/branch/evidence references: see existing story details and S562 board; no verified run or branch reference recorded.
