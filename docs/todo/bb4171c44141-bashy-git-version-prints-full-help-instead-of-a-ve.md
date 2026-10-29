---
id: bb4171c44141
kind: bug
title: bashy git --version prints full help instead of a version line
seq: 2743
status: todo
priority: p3
labels:
    - windows
created: 2026-10-27T17:56:43.040279Z
---

Spotted 2026-10-27 on a Windows host: 'bashy git --version' dumps the git help text. Agents probe versions with --version; print one version line like real git. Non-blocker.
