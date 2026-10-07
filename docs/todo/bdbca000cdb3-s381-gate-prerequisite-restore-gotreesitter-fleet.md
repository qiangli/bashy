---
id: bdbca000cdb3
kind: bug
title: 'S381 gate prerequisite: restore gotreesitter fleet-prepare mapping'
seq: 408
status: assigned
priority: p0
created: 2026-10-07T03:47:17.937694Z
weave: 18
assignee: codex-gpt5.6-sol
sprint: 381
sprint_id: 1a8fa6b8-96d8-5f96-bcfa-d01ecbb8005c
sprint_title: 'Bash# as the agent action language: evidence of record, uncovered gaps, first paired experiment'
---

Manager remote baseline make test fails at scripts/test-sibling-pins.sh: gotreesitter has no fleet-prepare repository mapping. Repro: sh scripts/test-sibling-pins.sh. go.mod and .sibling-pins already use permissive qiangli/gotreesitter fork; scripts/bootstrap-siblings.sh maps it, dag.md fleet-prepare omits it. Scope: smallest correct dag.md mapping repair; preserve pinned SHA and license boundary, no test weakening. Gate: existing sh scripts/test-sibling-pins.sh and inspect fleet-prepare matches canonical public fork. No full builds needed. Isolated worker, commit named file with supplied trailers and BASHY_AGENT; no push, manager integrates. Do not touch S1 hint or S7/S8 runtime files.
