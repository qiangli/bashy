---
id: fa286c7b7b6a
kind: bug
title: chat steer / mb delivery injects text+Enter into a live session that is showing a question dialog, answering it as if the operator did
seq: 3280
status: todo
priority: p1
created: 2026-11-21T08:38:49.458932Z
---

2026-11-21, conductor session claude+2 (interactive Claude Code, steerable). The conductor asked the operator 5 multiple-choice questions through the TUI's question dialog; every answer came back as the first (Recommended) option, and the operator reports none were theirs ('the console was stolen ... an issue for controlled pty'). Meanwhile mb posts addressed to claude+2 were delivered as 'steer' lines into the same PTY (bashy chat timeline: steer claude+2 '[mb] from ...' at 08:20:27, 08:21:54, 08:22:39, 08:29:07, 08:30:28, 08:31:35, 08:32:03, 08:33:27, 08:34:21Z). Hypothesis (unconfirmed): a steer writes text + Enter regardless of screen state; in a select dialog Enter picks the highlighted first option. Effect: false operator approvals were recorded on Sprints 677/688 (voided by comment) and a worker acted on one (muse local commit f6cc38ec, unpushed). Root cause first: reproduce with a disposable steerable session showing a dialog, confirm which bytes land and which choice is taken. Expected behaviour: delivery to a session in 'dialog' (or input-pending) state must queue (demote, never drop) until the dialog closes; never type into a dialog. Related: 6254c3c6 (live session state incl. dialog), yoke pkg/bus DEMOTE-NEVER-DROP, agentpty DialogGuard. Also observed: worker posts (muse, agy) and conductor steers are attributed to 'qiangli', and conductor sprint comments to muse-wonder-full.
