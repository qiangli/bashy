---
id: ddf0079eb063
kind: bug
title: A3b Shell defects reproducible without the licensed suite
seq: 3053
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T04:31:35.81564Z
weave: 13
assignee: codex-gpt6-sol
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T05:55:52.455462Z
closed_by: claude-opus5.5
---

Child of A3 (3d4d3d262bdd), filed for independent ownership; A3 acceptance unchanged. Source: docs/s561/a2-results.md (A2 run #9, branch agent/weave-issue-9). Baseline = frozen tree (bashy main == 5a4bd03f outside docs), standalone GOWORK=off. Rules: smallest causal repair at the owning layer; red test first, then green; no skip, weakened assertion, timeout increase or new feature; one separate C commit per fix; if a preserved patch exists on origin/s557-repair-stack-quarantined use it only after confirming it fixes the reproduced failure; fixes in sibling repos land in umbrella/<repo> then bashy pin — present the diff for manager review before merging; do not push. Never touch $HOME/projects/poc/dhnt. Commit trailers Sprint: #561, Story: <this>, Story-ID: <id>; BASHY_AGENT = you. Deliver a short docs/s561/<story>-results.md with red/green evidence. Scope (A2 I11/I12, vsc-pcts-run-status.md): I12 $! after a backgrounded subshell group yields a g<N> sentinel instead of a PID; I11 sh utility items — sh -s, -c/-s reading stdin, subshell-syntax-error exit status, PATH_MAX handling, async events. For each: write a public POSIX repro compared against GNU Bash 5.3 --posix/dash, classify real defect vs declared limitation vs environment. Only confirmed defects get a fix, in umbrella/sh (owning layer) with a focused regression test, then a bashy pin bump; present each sh diff for manager review before merge. Must keep make test-bash 86/86 and sh focused tests green.
