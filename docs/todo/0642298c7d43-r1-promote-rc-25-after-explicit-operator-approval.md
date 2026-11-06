---
id: 0642298c7d43
kind: task
title: R1 Promote rc.25 after explicit operator approval
seq: 3151
status: todo
priority: p1
labels:
    - release
created: 2026-11-06T21:50:00.657162Z
sprint: 580
sprint_id: 0cae3983-9725-5e31-b45d-3585e97e00de
sprint_title: Bashy checkpoint and release candidate v1.0.0-rc.25
---

Blocked on explicit operator approval of the gated candidate. Promote the same tested bytes (never rebuild) via promote.yml with the commit-pinned tier verdict; spot-check sha256 and the bashy-scratch digest against the -dev build; scripts/generate-install-channels.py and a native winget validate; bashy mb announcement. Not v1.0.0: no v1.0.0 before official POSIX certification.
