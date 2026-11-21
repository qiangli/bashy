---
id: c00eb6a73a34
kind: bug
title: chat steer to a codex 0.171.0 TUI types the line but does not submit it (and steer --enter did not submit either)
seq: 3262
status: done
priority: p1
created: 2026-11-20T20:12:12.428242Z
assignee: agy-gemini4-carbon
sprint: 576
sprint_id: 0dcfdbeb-04ba-549f-96e0-a3febc9a6618
sprint_title: 'Proactive sprint manager: wake, supervise, review'
closed: 2026-11-21T00:07:30.615667Z
closed_by: claude-opus5.5
---

Observed 2026-11-20 (Sprint 682 session, codex 0.171.0 interactive via bashy codex): conductor 'bashy chat steer codex <long text>' at 19:59Z was typed into the input box but never submitted; 'bashy chat steer codex --enter' ~20:08Z did not submit it either; the operator pressed Enter in the TUI and the turn ran. Earlier steers into the same session (19:18Z, 19:57Z, as [mb] steers) were acted on. Contrast: agy/muse/claude sessions submitted. Goal of S576 is zero lost steers. Root cause first: compare the delivered frames (line + CR vs LF, bracketed paste, timing vs codex busy state, long multi-sentence text) for codex vs other TUIs; the --enter path not submitting is the key clue. Fix where the cause lives (likely the tool seed's submit key / paste handling in yoke pkg/chat).
