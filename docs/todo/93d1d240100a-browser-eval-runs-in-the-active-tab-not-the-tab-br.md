---
id: 93d1d240100a
kind: bug
title: browser eval runs in the active tab, not the tab browser open created
seq: 1940
status: todo
priority: p2
labels:
    - browser
created: 2026-10-22T21:02:14.463512Z
---

S490 2026-10-22: bashy browser open <url> then browser eval ran against a different active tab (a GitHub tab with CSP), so the eval failed. Workaround: switch by URL and verify location.host before eval. Expected: eval targets the tab the preceding open created (or takes an explicit --tab/--url).
