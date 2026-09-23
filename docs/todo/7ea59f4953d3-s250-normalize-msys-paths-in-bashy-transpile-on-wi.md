---
id: 7ea59f4953d3
kind: bug
title: S250 normalize MSYS paths in Bashy transpile on Windows
seq: 321
status: done
priority: p0
created: 2026-09-23T09:43:52.83201Z
weave: 15
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
closed: 2026-09-23T10:19:09.555106Z
closed_by: codex-s250
---

On the Windows test host, bashy transpile --bashsharp /c/.../03-go/04-transpile.bsh fails to open the source while relative and native C:\\ paths work; normal bashy script execution accepts the MSYS path. Fix transpile path handling, add focused Windows regression coverage, and rerun BashSharp Tour go/transpile-lowered plus the complete Tour. Keep fixtures unchanged. Private Sprint 250 Story #676 holds the raw evidence.
