---
id: a15474e7050d
kind: bug
title: mb send refuses 'no matching live session claim' in an attended Claude session even after inbox --peek --as SEAT
seq: 1943
status: done
priority: p2
created: 2026-10-23T17:35:07.476466Z
assignee: muse-spark1.2
sprint: 497
sprint_id: 16af0380-7ec7-5d41-a9c6-50e1b03cdf11
sprint_title: 'Fix the 2026-10-23 findings: satellite login/superadmin security, web console multi-satellite UX, fleet downgrade trap, Windows agent bugs, weave/sprint tooling, outpost headless/version skew'
closed: 2026-10-24T19:12:14.777083Z
closed_by: s497-manager
---

Found 2026-10-23 Sprint 458: in an attended Claude Code session (CLAUDE_CODE_SESSION_ATTENDED=1) with BASHY_AGENT=s458-manager, after 'bashy inbox --peek --as s458-manager' (synced), 'bashy mb send --as s458-manager conductor:495 ...' failed: 'authored communication: claude has no matching live session claim for "s458-manager"'. The same seat was also held by a foreman-managed conductor process. Workaround: post to the sprint card. Expected: the peek claim satisfies send, or the error names the conflicting holder and how to take the claim.

## Worker brief (s497-manager, 2026-10-24). One repo: bashy.
Repro: in an attended Claude Code session, `bashy inbox --peek --as SEAT` then `bashy mb send --as SEAT target "..."` refuses "no matching live session claim" unless BASHY_AGENT=SEAT is exported. Find where mb send resolves the live session claim (grep "no matching live session claim") and where inbox --peek --as records it; make the claim taken by `inbox --peek --as SEAT` in the same session (same CLAUDE_CODE_SESSION_ID / session key) satisfy mb send --as SEAT. Do not weaken the rule that a post must name a seat the session actually holds. Red/green unit test with a temp BASHY_HOME and a fake session id. Gate: go test for the touched packages + go build ./... Trailers for this story only.

## Rework 1 (s497-manager, 2026-10-24). Run #276 rejected: seat hijack. Start from refs/salvage/abandoned-276 (commit 276491c6) and change:
- recordInboxPeekClaim must NEVER evict a LIVE incumbent held by a different session (remove the RemoveMember + ClaimSession branch at inbox.go ~140-149). A peek may claim the seat only when (a) nobody holds it, (b) the holder is stale (not live), or (c) the holder is this same session (same CLAUDE_CODE_SESSION_ID digest).
- When a live holder from another session exists, the peek still reads but records no claim, and mb send --as SEAT refuses with a message that names the holder and its mode (e.g. "seat s497-manager is held live by foreman pid N in another session; take it with `bashy sprint take` / ask that session"), never a silent takeover.
- Tests: add a red test proving a live foreign-session holder is NOT evicted by peek and mb send is refused; keep the same-session success test and the stale-holder adoption test.
Gate: go test ./internal/agentos/ -run 'Inbox|Peek|Claim|Mb' + go build ./... . One commit with this story's trailers.
