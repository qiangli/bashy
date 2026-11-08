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

2026-11-08 codex/muse/agy (operator logins copied into throwaway containers, removed after):
- codex 0.167.0, first run (60b38856): FAILED the intent — its web search summarized the page and it followed docs/install.md, so it installed bashy but never connected itself. Fixed in 5b34148b (raw-README pointer at the top of README and docs/install.md).
- codex rerun (5b34148b, fresh container): read the raw README; headless `exec` (approval never) chose option a itself and reported it; install, skill export, MCP (it re-added bashy with the absolute path), install-agent all ran. New session: skill loaded, bashy MCP connected, shell = bashy. PASS (no consent prompt is possible in headless exec).
- muse: BLOCKED — the container's current muse 1.4.14 rejects the host's auth.json (schema_version 2, written by host muse 1.4.3). Needs `muse login` (device code) inside the sandbox.
- agy 1.3.2: BLOCKED — copied ~/.gemini oauth files are not enough; it starts an interactive Google OAuth flow. Needs one interactive `agy` sign-in inside the sandbox.
