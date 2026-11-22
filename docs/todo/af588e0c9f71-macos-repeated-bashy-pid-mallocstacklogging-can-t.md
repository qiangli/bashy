---
id: af588e0c9f71
kind: bug
title: 'macOS: repeated ''bashy(PID) MallocStackLogging: can''t turn off malloc stack logging because it was not enabled'' spam in agent consoles'
seq: 3288
status: todo
priority: p2
created: 2026-11-21T15:39:53.295621Z
---

Operator 2026-11-21: agy/codex consoles show repeated lines 'bashy(46884) MallocStackLogging: can't turn off malloc stack logging because it was not enabled.' with rising PIDs, interleaved with the TUI border/status line, i.e. short-lived bashy processes spawned repeatedly (status line, hook or watcher) in an environment that carries a MallocStackLogging variable while libmalloc is asked to disable it. Concern: token cost if the lines land in an agent's captured command output (display-only lines cost nothing). Root cause first: identify which parent environment sets MallocStackLogging/MallocStackLoggingNoCompact (agent TUI, launcher, bashy chat spawn env), which periodic process spawns bashy, and whether any agent tool result contains the line. Fix where it lives: strip the variable from spawned child environments (bashy agent launch / chat), or don't set it; add a test that a spawned bashy emits nothing on stderr for a no-op on macOS.

Root cause, 2026-11-22: environment, not bashy code. The spawning process is the Claude Code status line `bashy model quota capture --to ...`, which Claude Code re-runs on every status refresh; its stdout "Opus 5.5" is the text interleaved with the spam. No live process carries a MallocStackLogging variable, and the same command run from a normal shell prints nothing on stderr, so the message comes from how the agent TUI spawns status-line children. That child's stderr reaches the terminal. The lines never entered any agent tool output, so there is no token cost. Fix applied in the operator's local agent settings: the status-line command ends in `2>/dev/null` (stdout unchanged). Remaining: check the codex/agy status hooks for the same pattern if their consoles still show it.
