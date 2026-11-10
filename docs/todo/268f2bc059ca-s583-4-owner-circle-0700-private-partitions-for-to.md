---
id: 268f2bc059ca
kind: feature
title: 'S583.4 Owner circle: 0700 private partitions for todo/kb/memory/sprint/mb/chat/ask/handoff, never shared'
seq: 3135
status: todo
priority: p0
labels:
    - boundary
    - owner
created: 2026-11-06T21:45:55.779213Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / family / friend / team / guest / public) across all of bashy'
---

Owner circle part 2: the owner's private space for everything, not only secrets. Via yoke/pkg/scope.Resolve, every host store gets a dedicated 0700 owner partition (~/.bashy/<tool>/owner/): todo, kb, remember/recall memory, notes, sprint items, mb notes-to-self, chat and ask history, handoffs. owner data is never written to a repo ring (todo/kb --repo with circle owner refused) and never carried by mb, meet, cross-host relay, feed, handoff, mirror, sphere or cloudbox (even between Owned: true outposts under the same Cloudbox email). Visible only to the owner human at an interactive TTY; never to family, friend, team, guest, other hosts, or ungranted agents. Per-session agent grant requires interactive TTY `bashy circle use owner` through the human approve gate; secrets stay handle-only, model endpoints are restricted to owner-bound local endpoints (e.g. local ollama), and all output stays in owner.
Depends on: S583.2, S583.3.
Acceptance: owner todo/kb/memory invisible from every other circle and to ungranted agents; repo-ring writes refused; every transport refuses owner data; grant expires with the session.
