---
id: 20f1853cb593
kind: task
title: 'B5 one execution plane, observe-only (PR1): front-door Dispatch through the wireExec middleware chain'
seq: 263
status: todo
priority: p2
created: 2026-09-12T23:03:30.263014Z
sprint: 161
---

One execution plane, OBSERVE-ONLY (script-execution plan PR1). Route front-door Dispatch() verbs (internal/agentos/agentos.go:332) through the same wireExec middleware chain as shell-resolved commands — telemetry -> audit -> execlog -> advisor -> learn — so bashy skill run, bashy chat, bashy dag are RECORDED like any other action. No policy, no deny, no envelope change, no behavior change to any verb: the chain only observes on this path.
Gate: BASHY_AUDIT=1 bashy skill show conductor appears in bashy inspect audit tail; execlog has the record; go test ./internal/agentos/...; e2e dispatch gate unchanged.
If the change cannot be made observe-only within the story, STOP and record why — this story may land as a finding instead of code.
