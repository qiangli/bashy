---
id: 5b392fd5198d
kind: task
title: Pin linked-libc Coreutils locale provider in certified one-file Bashy
seq: 406
status: done
priority: p0
created: 2026-10-03T04:15:43.787693Z
assignee: codex-gpt6-sol
sprint: 355
sprint_id: 3a83ff48-7f8b-5be4-b0e6-e146762b2573
sprint_title: Profile D residual blocker triage and targeted closure
closed: 2026-10-04T07:13:50.12649Z
closed_by: codex-gpt6-sol
---

Bashy standalone sibling pins must name Coreutils b24d3a76 with certified cgo locale providers. Verify Linux one-file static build, signal symbol, POSIX mode, locale cold-start stress, and Bash# Go roots remain unchanged.

## Sprint 355 acceptance evidence 2026-10-04

Linked-libc locale pin is merged at 5101d84 and 8f0e68f; guarded full6 static candidate completed awk and all 117 sets without caps. Historical raw journals and any pending formal certification decisions are unchanged.
