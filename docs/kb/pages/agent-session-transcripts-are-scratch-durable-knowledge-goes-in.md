---
id: 01a1c233-fcd9-715b-8d58-74b0cd77328f
seq: 19
form: page
type: decision
title: Agent session transcripts are scratch; durable knowledge goes in bashy kb
description: 'Where agents keep knowledge vs transcripts, and when transcripts may be deleted — WHEN ending a session, cleaning disk, or adding a new agent tool (keywords: transcript, session, history, codex, claude, agy, muse, zcode, ycode, genie, opencode, retro, cleanup, disk, retention)'
tags:
    - agents
    - kb
    - retention
    - cleanup
status: candidate
evidence: '2026-11-09: operator decision after a dev-box disk cleanup deleted codex sessions; durable state was already in sprint cards, docs/todo and kb'
source:
    tool: claude
created: "2026-11-09T21:25:49Z"
---

Applies to every agentic tool on a host: claude, codex, agy, muse, zcode, ycode/genie, opencode, and any future or custom agent CLI.

**Rule: an agent's session transcripts are disposable scratch. Durable knowledge lives in bashy, never only in a transcript.**

Each tool keeps its own private transcript/state store, for example `~/.codex/sessions`, `~/.claude/projects`, `~/.gemini`, `~/.local/share/muse`, `~/.local/share/opencode`, `~/.ycode`. These stores are per-tool and per-host, other agents cannot read them, and they grow to gigabytes. Treat them like a build cache: they exist only so that tool can resume or replay a conversation.

Where knowledge goes instead (shared by every agent, travels with the clone):

| What | Where |
|---|---|
| reusable lesson, gotcha, runbook, decision, fact | `bashy kb add` (repo store when it is about that repo) |
| work state, plan, resume brief | sprint card: `bashy sprint checkpoint`, `bashy sprint comment` |
| a task or follow-up | `bashy todo add` (committed `docs/todo/`) |
| a message to another agent or a human | `bashy mb send` / `bashy inbox` |
| per-tool preferences | that tool's own memory file (optional; never the only copy of shared knowledge) |

Duties:

1. **Every agent, before it ends a session:** run `bashy kb retro` and record what is durable. A session that learned something and only left it in its transcript lost it.
2. **Before deleting any agent's transcripts** (disk cleanup): this is safe once the retro has been done, and you don't need to ask about the content. Keep the last day or two while that tool is running so live conversations can still be resumed, and say that resuming older conversations is no longer possible. Delete the transcript files only: leave a tool's config, auth, prompt history and memory files alone unless the operator names them.
3. **New or custom agent tools:** follow the same contract. Register the tool's transcript directory as scratch in its adapter or README, and use bashy kb/sprint/todo/mb for anything that should outlive the session.
