---
id: 223b119f24c4
kind: bug
title: S250 fix Windows Rust island Cargo path spelling
seq: 323
status: assigned
priority: p0
labels:
    - windows
    - bashsharp-tour
created: 2026-09-23T10:11:49.670485Z
weave: 17
assignee: qiangli
sprint: 250
sprint_id: c912e608-edfe-59b8-bd36-a98f6dad1634
sprint_title: Validate Go by Example, Go Tour and BashSharp Tour on three hosts
---

BashSharp Tour islands/rust fails on Windows current Bashy candidate. Cargo panics because a dependency source is spelled /c/Users/... with a backslash segment and is not a native absolute path. Reproduce the unchanged island case, normalize the relevant Rust/Cargo path at the Bashy provisioner boundary, keep the pinned transcript and timeout, and rerun Windows Tour. Sprint 250 Story #676.
