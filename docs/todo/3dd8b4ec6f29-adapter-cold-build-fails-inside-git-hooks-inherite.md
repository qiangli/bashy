---
id: 3dd8b4ec6f29
kind: bug
title: Adapter cold build fails inside git hooks (inherited GIT_DIR breaks go VCS stamping)
seq: 3299
status: todo
priority: p1
labels:
    - adapters
created: 2026-11-22T13:08:39.904914Z
sprint: 692
---

Symptom (2026-11-22, dev box, freshly installed bashy dev build): an umbrella `git commit` failed in the commit-msg hook (`bashy sprint commit-msg`) with "error obtaining VCS status: exit status 128 / Use -buildvcs=false to disable VCS stamping". After the yoke/sprint adapter binary had been built once outside the hook, the same commit passed.

Likely cause, not yet confirmed by a repro: the first `bashy sprint` call after install cold-builds the yoke/sprint adapter (`go build` in the adapter's source directory). Inside a git hook the environment carries GIT_DIR/GIT_INDEX_FILE for the umbrella, so go's VCS stamping runs git against the wrong repository and fails.

Fix direction: adapter .bsh builds pass `-buildvcs=false` (adapter binaries do not need VCS stamps) or strip GIT_* from the build environment. Unit test: a cold adapter build with GIT_DIR pointing at another repository succeeds.
