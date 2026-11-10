---
id: 92b67dc42fd0
kind: doc
title: 'S583.1 Design of record: owner by default, circles are member attributes, enforce at egress'
seq: 3132
status: assigned
priority: p0
labels:
    - boundary
    - design
created: 2026-11-06T21:45:55.324575Z
assignee: claude-opus5.5
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control (KISS): owner by default, circles from the member list, checked where data leaves'
---

Design of record, one page: docs/boundary.md (one line in docs/INDEX.md, abstract in docs/README.md). Design only. KISS rescope 2026-11-09.

Model (owner by default, share by choice):
- Everything on an owner's host is the owner's. Nothing is labelled, partitioned or migrated on disk; owner-only is the default, so the "owner circle is absolute" requirement is met by refusing every member-facing egress that has no explicit grant.
- A circle is an attribute of a MEMBER, not of data: the circle the owner gives each email on their Cloudbox member list (family, friend, team, guest). public = anyone. Source of truth is the Cloudbox list (ticket a99e88ac: validated circle field, /api/v1/members, reverse view); bashy keeps no parallel list.
- A share grant names an email or a circle; a circle grant resolves to the current members of that circle at check time, so removing a member or changing their circle revokes at once (within the list's cache TTL).
- Secrets never leave: agents use them by handle only; known values are redacted at egress; plaintext reveal needs the interactive TTY approve gate.
- Enforcement only at egress points. Inventory them in the doc with today's check: Cloudbox app/host shares (already per member email), the LLM door (peers), cross-host mb/bus, meet/room, handoff, feed, public git push (scrub-gate). The MVP enforces the member-facing ones that exist today; the rest are listed as follow-ups.
- Agents on the owner's host act as the owner; an agent that serves members (e.g. through the LLM door) sees only what was granted.

Decisions to record: owner by default (above); the name "circle"; mutuality (from ticket a99e88ac; recommendation: accept adds the reverse row); what an offline host does with a stale list (last known list, unknown email = not a member).
Deferred (unlinked from this sprint, label boundary-later): per-circle store partitions, ceiling-locked BASHY_CIRCLE sessions, per-circle credential/CLI profiles and model routing, execution isolation, observability.
Prior art: human-lane 101f7ec3b611 (I14/I15), S513 invite-only sharing, Sprint 606 pair-once.
Public repo: placeholder names only, hosts by role.
Acceptance: doc merged with the decisions; story list matches.
