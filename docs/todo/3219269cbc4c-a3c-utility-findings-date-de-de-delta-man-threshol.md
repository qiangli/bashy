---
id: 3219269cbc4c
kind: bug
title: 'A3c Utility findings: date de_DE delta, man threshold, editor EOF diagnostic'
seq: 3054
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T04:31:36.168911Z
weave: 12
assignee: muse-conductor
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T05:17:06.512965Z
closed_by: claude-opus5.5
---

Child of A3 (3d4d3d262bdd), filed for independent ownership; A3 acceptance unchanged. Source: docs/s561/a2-results.md (A2 run #9, branch agent/weave-issue-9). Baseline = frozen tree (bashy main == 5a4bd03f outside docs), standalone GOWORK=off. Rules: smallest causal repair at the owning layer; red test first, then green; no skip, weakened assertion, timeout increase or new feature; one separate C commit per fix; if a preserved patch exists on origin/s557-repair-stack-quarantined use it only after confirming it fixes the reproduced failure; fixes in sibling repos land in umbrella/<repo> then bashy pin — present the diff for manager review before merging; do not push. Never touch $HOME/projects/poc/dhnt. Commit trailers Sprint: #561, Story: <this>, Story-ID: <id>; BASHY_AGENT = you. Deliver a short docs/s561/<story>-results.md with red/green evidence. Scope (A2 I10/I2/I3): date +34 de_DE.iso88591 locale delta (reproduce with the de_DE fixture locale vs GNU date); man threshold failure (man is a mandatory Go applet, cmds/man); four EOF/editor failures — bounded unchanged-PTY/locale diagnostic only, no code until the cause is confirmed. Fixes land in umbrella/coreutils as separate C commits with focused tests (go test ./cmds/<name>/...), then bashy pin bump; present diffs for manager review before merge. Also fold in the stale help comment coreutils/cmds/posixgate/posixgate.go:26 (94/14/8 -> 96/14/6, 86/22/8 -> 88/22/6) only if posixgate is otherwise touched.
