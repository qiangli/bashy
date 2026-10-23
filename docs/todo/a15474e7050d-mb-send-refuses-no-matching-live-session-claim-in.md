---
id: a15474e7050d
kind: bug
title: mb send refuses 'no matching live session claim' in an attended Claude session even after inbox --peek --as SEAT
seq: 1943
status: todo
priority: p2
created: 2026-10-23T17:35:07.476466Z
---

Found 2026-10-23 Sprint 458: in an attended Claude Code session (CLAUDE_CODE_SESSION_ATTENDED=1) with BASHY_AGENT=s458-manager, after 'bashy inbox --peek --as s458-manager' (synced), 'bashy mb send --as s458-manager conductor:495 ...' failed: 'authored communication: claude has no matching live session claim for "s458-manager"'. The same seat was also held by a foreman-managed conductor process. Workaround: post to the sprint card. Expected: the peek claim satisfies send, or the error names the conflicting holder and how to take the claim.
