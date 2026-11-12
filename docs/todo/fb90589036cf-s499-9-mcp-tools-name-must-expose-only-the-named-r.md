---
id: fb90589036cf
kind: bug
title: 'S499.9 mcp: --tools NAME must expose only the named registered commands, not the whole ring'
seq: 3226
status: todo
priority: p2
labels:
    - mcp
created: 2026-11-12T17:05:34.659433Z
sprint: 499
sprint_id: c3c45f32-63b5-5575-980f-299a44a561f3
sprint_title: 'Content creation with bashy: YC videos and a local-first author-to-reader showcase'
---

S499.9 leftover (code half; tracking story S499.9 124ec3cbaa4e lives in the operator's private store).

Bug: `bashy mcp tools --tools record` lists `record` plus EVERY registered command (16 on the dev box, 2026-11-12, rc.31-dev 9027d07a). `bashy mcp serve --tools NAME` uses the same options, so a server asked for one tool exposes every registered command.
- Expected: a name-list profile exposes exactly the named tools.
- Cause: in internal/agentos/mcp.go mcpOptions, for a non-default profile the Registered func returns the whole registered ring plus the selected verbs (mcp.go:284-293). The name list filters only core tools and verbs, never the ring.

Fix:
- For a name-list profile (not "default" or "all"), keep only ring entries whose Name is listed, and remove those names from opts.Tools so yoke's core lookup does not see them.
- An unknown name still fails as today (mcp_test.go:65).
- `--tools all` and `default` behaviour unchanged.

Tests:
- next to mcp_test.go:65: a fixture ring with two registered commands; `--tools one` lists only that one; `--tools one,two` lists both
- an unknown name errors
- `default` and `all` still list the whole ring
Cross-build `GOOS=windows`.
