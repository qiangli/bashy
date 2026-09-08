---
id: 8537ec540aab
kind: task
title: 'S117-07: transpile CLI source diagnostics and standalone artifacts'
seq: 246
status: done
priority: p1
created: 2026-09-07T17:54:06.128715Z
assignee: agy-gemini3.1
sprint: 117
closed: 2026-09-08T00:19:01.242261Z
---

Depends S117-02; final proof requires S117-03 through 06. Provide bashy transpile --bashpp INPUT -o OUTPUT.go using public sh lowering API. Stable inspectable source/maps, deterministic errors and fresh-directory go build with recorded deps. Preserve argv/stdin/stdout/stderr/status and standalone command/tool contracts. Parent 59bc1d4ea772.
