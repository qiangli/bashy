---
id: 15907111b30a
kind: task
title: Dev-host install, smoke and release notes for rc.25; promotion needs operator approval
seq: 3131
status: todo
priority: p1
labels:
    - release
created: 2026-11-06T20:10:28.175207Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

After the candidate is gated: on the dev host run make install (canonical ~/.local/bin/bashy) from the frozen candidate and smoke the installed PATH binary (bashy --version, bashy sprint show, bashy agent list, bashy model, a dag smoke). Record installed path and results. Draft release notes using docs/public-claims-evidence.md checks (actual candidate tag, Bash 5.3, Go 1.27/go1.27.2). Propose promotion to the operator with the evidence; do not tag beyond -dev or publish a release without explicit operator approval. No v1.0.0 before official POSIX certification.
