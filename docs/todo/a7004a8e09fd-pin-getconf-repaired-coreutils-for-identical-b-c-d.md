---
id: a7004a8e09fd
kind: chore
title: Pin getconf-repaired Coreutils for identical B/C/D Bashy build
seq: 384
status: todo
priority: p0
created: 2026-10-01T14:00:23.065925Z
sprint: 341
sprint_id: 5f262cbb-e61a-5a5e-8361-c60190adf78f
sprint_title: 'POSIX certification: base XCU claim, pure Go, Linux x86_64 - fresh baseline, failure list, final run'
---

Bashy .sibling-pins still names Coreutils 875e78d, while Sprint341 getconf TP12 repair is in pushed Coreutils 5314a57. Update only the Coreutils pin, run Bashy build/three-OS CI and preserve the unrelated dirty story file. Final B/C/D shells and Go multicall must be built from this one Bashy commit and copied byte-identically.
