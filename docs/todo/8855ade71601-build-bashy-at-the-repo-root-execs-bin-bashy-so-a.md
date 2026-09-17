---
id: 8855ade71601
kind: task
title: 'build: ./bashy at the repo root execs bin/bashy, so a hand ''go build -o ./bashy.real ./cmd/bashy'' is silently ignored (a fixture ran a stale binary until make build); make the wrapper warn when a newer *.real/bashy sits beside it, or document that make build is the only build path'
seq: 303
status: todo
priority: p3
created: 2026-09-17T12:40:38.346636Z
---
