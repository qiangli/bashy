---
id: 0570b661d87a
kind: chore
title: Integrate official runner native and audit gates preserving worker commits
seq: 1923
status: done
priority: p1
created: 2026-10-13T17:15:20.733202Z
assignee: s436-codex-manager
sprint: 436
sprint_id: 20ad6f7b-d3d3-5ebf-973c-0bbb999db96f
sprint_title: 'Sphere regression recovery: p2p sharding + local inference back on, bashy-built engines'
closed: 2026-10-13T17:16:53.606723Z
closed_by: s436-codex-manager
---

Merge Bashy worker commits 9f41a541 (native protocol/network) and b9a34015 (audit) sequentially on main, preserving each commit as a parent. Resolve generated workflow overlap, rerender, run focused recipe/audit/native checks. Keep audit story submitted and hosted release blocked until compiled C/C++/Metal input review and hosted evidence. Commit/push integration with Sprint/Story/Story-ID trailers; do not mutate unrelated shared checkout files.
