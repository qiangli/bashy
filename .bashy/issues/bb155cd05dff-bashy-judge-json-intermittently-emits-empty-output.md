---
id: bb155cd05dff
kind: bug
title: bashy judge --json intermittently emits empty output (0 bytes, exit 0)
status: open
reporter: qiangli
created: 2026-07-13T00:04:20.264801Z
---

During gating on dragon-2, 'bashy judge --diff main...HEAD --agent claude-opus --stage code --json' produced ZERO bytes on stdout and stderr (exit 0) on multiple invocations: run #13's first attempt (empty; retry succeeded, 3857 bytes) and run #12 TWICE (both empty). The SAME judge in PLAIN text mode (no --json) for #12 worked immediately and returned a full APPROVE verdict. Runs #11 and #16 --json succeeded first try.

So the failure is intermittent and appears more likely in --json mode: the judge exits 0 having written nothing, which an automated gate would misread as 'no findings / empty diff'. A judge that silently emits nothing on exit 0 is unsafe for 'merge only if judge != reject' automation — empty must be a non-zero exit or a retry. Likely an underlying LLM call returning empty/timing out with the JSON formatter swallowing it. Add: non-empty-output assertion + retry, or fail closed.
