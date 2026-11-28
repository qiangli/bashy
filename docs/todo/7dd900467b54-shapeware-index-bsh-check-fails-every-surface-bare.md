---
id: 7dd900467b54
kind: bug
title: 'shapeware/index.bsh check fails every surface: bare conform resolves to bashy''s built-in conform, which has no spec subcommand'
seq: 3348
status: todo
priority: p2
labels:
    - shapeware
created: 2026-11-28T16:08:29.448361Z
---

Found in Sprint 745 on the Windows test host's Linux lane, 2026-11-28: bashy --bashsharp shapeware/index.bsh check prints 'Error: unknown command "spec" for "conform"' and FAIL for agent-identity, shape and hitl. Same result at bashy origin/main b65f6960 (baseline), so it is not from Sprint 745. conform is not on the lane PATH, so the bare name inside the Bash# script reaches bashy's built-in conform (the suite tool), not the shapeware judge. Sprint 744 recorded the index check passing earlier the same day; find what changed (bisect bashy between a5b5a943 and b65f6960). Per-folder lint 'bashy conform spec check DIR' is clean for all three. Likely fix: index.bsh calls the judge as "${BASHY:-bashy} conform".
