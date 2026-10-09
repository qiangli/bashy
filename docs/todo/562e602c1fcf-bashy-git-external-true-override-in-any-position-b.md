---
id: 562e602c1fcf
kind: enhancement
title: bashy git --external=true override in any position; bashy commands needing full git use it
seq: 419
status: todo
priority: p1
labels:
    - git
created: 2026-10-09T05:19:58.879867Z
sprint: 404
sprint_id: d0936923-9388-5532-b329-e23e58422343
sprint_title: bashy git -C regression and canonical reinstall
---

Operator decision 2026-10-09: bashy git stays NATIVE by default (the Sprint 252 one-door, internal/agentos/git.go); --external=true (or --external) overrides to the full host git; other bashy commands that require full git support must use it.

Acceptance: (1) `--external=true`, `--external=false` and `--external` parse in any position, including after git global options (`bashy git -C DIR --external=true rebase ...`), with a test. (2) Audit every place bashy (and yoke code bashy runs) invokes git on behalf of a command that needs full git: Go exec of "git", dag.md/Bash# fences and gates that call `git`, weave/sprint/mod sync/worktree helpers. Where the call can land on the native one-door (e.g. the interpreter's git builtin or the agent-shell `git` function) and the verb needs full git, make it use `bashy git --external=true` or the host git binary explicitly. Go code that execs the host git binary directly is already full git; list it, do not change it. (3) A table in the final report: caller -> path (native/external/host) -> needs full git? -> changed?. (4) Focused tests for each changed caller; no behaviour change for native-capable verbs.

BRIEF (Sprint 404, conductor claude-opus5.5): starts after the git -C story merges; branch agent/s404-git-external from origin/main; BASHY_AGENT = your injected identity; trailers as the final paragraph: Sprint: #404 / Story: #419 / Story-ID: 562e602c1fcf. Do not push main. Short focused tests only. Hard limit 45 minutes.
