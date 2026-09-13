---
id: 6970f05a136e
kind: task
title: B2 conductor skill PLAN/RETRO use kb context / kb note add --candidate / kb observe; Claude Code + Codex + bashy chat hook recipes under skills/
seq: 267
status: assigned
priority: p2
created: 2026-09-13T01:31:42.15962Z
weave: 10
assignee: qiangli
sprint: 163
---

Goal: the conductor skill and the third-party harness recipes use the stage verbs structurally.

- bashy/skills/conductor: PLAN calls kb context --for "<story title>" --rings repo,host (instead of an instruction to "check kb"); RETRO calls kb note add --candidate --episode <sprint-run> and kb observe for the gate outcome; validate happens only through kb validate --from-gate.
- Recipes documented under skills/ (not bashy code): Claude Code hooks (UserPromptSubmit -> kb context; Stop -> kb note add --candidate), Codex, and a plain bashy chat agent — same commands, same flags, to demonstrate the design is harness-agnostic.
- Any YAML agent: the ycode example (Y2) is referenced, not duplicated.

Gate: skill lint/verify passes; a dry run of the conductor PLAN step on a scratch sprint emits a kb context block; docs contain no real hostnames/user ids.
Depends on: B1.
