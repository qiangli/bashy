---
id: d23615a4130e
kind: task
title: S168.4 print the canonical ref wherever a record is listed (todo list, sprint show, kb list, mb --history, meet); JSON gains ref; script/e2e-refs.sh gate
seq: 276
status: todo
priority: p2
created: 2026-09-13T22:56:21.767085Z
sprint: 168
---

S168.4 (C1 coreutils renderers; C2 dhnt script/e2e-refs.sh). Plan: dhnt docs/sprint-168-master-execution-plan.md, D6/D8, traps 7+8+9.
C2 FIRST, in PLANNED mode (e2e-agent-inbox.sh discipline: PASS/FAIL/PLANNED + ratchet; hermetic scratch HOME; guarded rm -rf): mint one record of each of the 15 kinds without a model (kb add, todo add, sprint create, weave add, meet open, mb post, bus publish, agent/tool/model/skill/person add, hosts/<name>.yaml drop, BASHY_EPISODE run, role via steward/whois); cite all from one kb note; assert bashy define <ref> and <verb> show --links resolve each in both spellings; create the run: basename collision on purpose and assert the error. episode may stay PLANNED only if execlog cannot be relocated hermetically (D8), never a synthetic pass.
C1: text prints the ref at the listing id width (todo:b3d71e5c), show prints full, every --json gains full ref: todo list/show, sprint show, kb list, mb --history, meet read/show, weave list, bus watch --json, whois. No text column moves (golden diffs).
