---
id: f154394d1a9b
kind: feature
title: 'S583.11 Share points check members: LLM door peers bound to member email or circle'
seq: 3142
status: done
priority: p1
labels:
    - boundary
    - llm
created: 2026-11-06T21:45:56.843988Z
assignee: codex-gpt5.6-sol
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
closed: 2026-11-10T12:53:05.666095Z
closed_by: claude-opus5.5-s606
---

Share points check members. KISS rescope 2026-11-09. Depends on: S583.2.
- LLM door: a door peer may carry a member email (`bashy llm peer` add/set --email); `bashy llm peer` refuses an email that is not an active member; a request from a peer bound to an email that is no longer a member is refused (403, plain reason). S602 limits and usage stay keyed on the same peer record; per-peer model grants (todo 0a13a9da) attach to it later. Peers without an email keep working unchanged (owner's own tools, legacy).
- Grant to a circle: `bashy llm peer add --circle family` style grants resolve through S583.2 (`bashy member`) at check time.
- Cross-host mb/bus and meet/room: if a post or room reaches another account's host today, the same member check applies; if not, record "owner-only today" in the S583.1 inventory and stop.
- Cloudbox app/host shares are already per member email: no change.
Acceptance: member peer 200, non-member add refused, removed member 403, circle grant follows a circle change, legacy peer unchanged; S602 limits still apply.
