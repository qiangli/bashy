---
id: 3dd8b4ec6f29
kind: bug
title: Adapter cold build fails inside git hooks (inherited GIT_DIR breaks go VCS stamping)
seq: 3299
status: assigned
priority: p1
labels:
    - adapters
created: 2026-11-22T13:08:39.904914Z
assignee: codex-gpt6-astra
sprint: 692
---

RESCOPE (operator 2026-11-22, Sprint 692 plan umbrella docs/plan-sprint-692-release-thin-bashy.md): p1, owner codex, same run as 42f79daa (second commit). Fix direction below: adapter builds pass -buildvcs=false (adapter binaries need no VCS stamp); unit test: a cold adapter build with GIT_DIR pointing at another repository succeeds. TEST RULE (operator 2026-11-22): build + the smallest targeted unit tests for the changed packages only, then submit. No go test ./..., no gates, no integration per story or per commit. The one full run is the rc.40 candidate gate (458628c0) at sprint close. Start on current heads (bashy mod drift; rebase before submit). Trailers: Sprint/Story/Story-ID + Agent; bashy commit+push first, then the umbrella pin.

Symptom (2026-11-22, dev box, freshly installed bashy dev build): an umbrella `git commit` failed in the commit-msg hook (`bashy sprint commit-msg`) with "error obtaining VCS status: exit status 128 / Use -buildvcs=false to disable VCS stamping". After the yoke/sprint adapter binary had been built once outside the hook, the same commit passed.

Likely cause, not yet confirmed by a repro: the first `bashy sprint` call after install cold-builds the yoke/sprint adapter (`go build` in the adapter's source directory). Inside a git hook the environment carries GIT_DIR/GIT_INDEX_FILE for the umbrella, so go's VCS stamping runs git against the wrong repository and fails.

Fix direction: adapter .bsh builds pass `-buildvcs=false` (adapter binaries do not need VCS stamps) or strip GIT_* from the build environment. Unit test: a cold adapter build with GIT_DIR pointing at another repository succeeds.
