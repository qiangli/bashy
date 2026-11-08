---
id: f387d1a0c5d9
kind: task
title: 'Cold start from the real GitHub URL: claude and codex'
seq: 3185
status: todo
created: 2026-11-08T01:46:27.984629Z
sprint: 591
sprint_id: dc240e08-28b0-5592-85a1-15e3d650c716
sprint_title: 'README as agent bootstrap: human hook + agent skill, one consent gate'
---

Fresh container, non-root, no bashy: agent told 'Set up bashy from https://github.com/qiangli/bashy'. Pass = one consent ask, install exit 0, skill+MCP+shell wired, new login session reports skill loaded, MCP connected, shell bashy. Draft-mounted run on 2026-11-07 passed for claude.

2026-11-08 progress: claude PASSED from the real URL after 60b38856 (R1 one consent ask; R2 install, path, skill, MCP, install-agent exit 0; R3 MCP connected, shell bashy). codex BLOCKED: both vault OpenAI keys (dragon-openai, openai) return "Quota exceeded"; needs a funded key or the operator's codex subscription login in the sandbox.
