---
id: 6254c3c64248
kind: feature
title: 'Live session state detection for managers: rate-limited, idle-awaiting-instruction, input-pending (needs Enter), dialog, working - surfaced without operator eyes'
seq: 3272
status: todo
priority: p1
created: 2026-11-20T23:26:08.610507Z
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
---

Operator request 2026-11-20 after watching three stalls the conductor could not see: (a) muse hit a 429/rate limit; (b) codex finished a turn with a recap and sat idle waiting for its next instruction (not a CR/LF problem); (c) agy sat with typed input waiting for Enter. The conductor's monitor only watches the board, so all three looked like 'working'.
Existing pieces to build on (no new transport): fleet usage_signals (only codex declares them; applied to weave runs, not live chat sessions), agentpty.ScreenText + DialogGuard gate detection, room status events, sprint await event typing, bashy inbox --until-count.
Shape: one classifier per live session from the PTY screen + output activity: working | idle-ready (prompt visible, no output for N s after a turn) | input-pending (text in the input box, not submitted) | rate-limited (usage_signals match; reset time when known) | dialog (recognized gate) | exited/error. Per-tool patterns come from the tool seeds (declare usage_signals for muse and agy too, measured, never guessed; prompt/input-box markers). Surface: 'bashy chat sessions --json' gains state, since, reason; transitions emit room status events; 'sprint await' and 'bashy inbox --until-count' can wake a manager on 'idle-ready', 'input-pending' and 'rate-limited' for sessions working the manager's sprints, each with a suggested action (send next instruction / chat steer --enter / wait until reset or reassign). Tests: screen fixtures per tool for each state; a 429 fixture from muse; transition events. Root cause first for any tool whose idle/input state cannot be distinguished from the screen.
