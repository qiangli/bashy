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

## Evidence (2026-11-20, from the live sessions)

**muse (Muse Code 1.4.4):** the session log (the card's `log_path`) shows,
verbatim:

    model failed: API error 429 [request_id=<id>]: Subscription quota exhausted. Your usage window resets at 2026-11-20T23:37:18Z. (rate_limit_error)

That is enough for a muse usage_signal (kind exhausted, reset = RFC3339
timestamp). After the reset, muse delivered its story and then sat IDLE with
the stale 429 lines still on screen. A classifier must order events in time:
the latest state wins, not any 429 visible in the scrollback.

**codex (0.171.0):** idle at a `/recap` screen that proposes "next: <action>",
waiting for the operator or manager to say go. That is the 'idle-ready' state
with a proposed next step. Surface the proposal text to the manager so it can
approve it, or approve it automatically under a sprint directive.

**agy:** sat with typed input waiting for Enter ('input-pending').

**Source for detection:** each session's log_path transcript on the room card
already captures all of this, with no new capture needed.

## Operator correction (2026-11-20): window limit, not exhaustion

muse's 429 says "Subscription quota exhausted", but it was a SHORT-WINDOW rate
limit. Right after the reset the operator read 3% used of the current window
and 47% of the weekly limit. So the classifier and the watermarks must:
- classify by window: derive it from how far away the reset is, as codex's
  5h-vs-week usage_signals do. Never take the vendor's wording at face value.
- track at least two measured levels per subscription: current window % and
  weekly %.

Today muse's /status shows only cumulative session tokens, and bashy's
account monitor has no muse source ("0 accounts, 4 unknown"). Find where the
current-window and weekly % are measurable (a vendor usage view or endpoint)
before designing the source. Never guess.
