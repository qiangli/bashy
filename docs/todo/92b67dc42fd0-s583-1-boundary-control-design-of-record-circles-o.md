---
id: 92b67dc42fd0
kind: doc
title: 'S583.1 Boundary control design of record: circles (owner/family/friend/team/guest/public)'
seq: 3132
status: todo
priority: p0
labels:
    - boundary
    - design
created: 2026-11-06T21:45:55.324575Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / family / friend / team / guest / public) across all of bashy'
---

Design of record for Sprint 583 boundary control. Deliver docs/boundary.md (abstract in docs/README.md if present, one line in docs/INDEX.md). Design only, no code.

Problem: a human knows their audience (when in Rome, do as the Romans do): family matters stay out of the office, teammates do not see private life, account secrets are shown to nobody. On an outpost or standalone bashy host, everything runs under one OS user (~/.bashy and ~/.config/bashy): principal owner is hard-coded "local" (yoke/pkg/principal/principal.go), scope stops at repo|user|agent (yoke/pkg/scope/scope.go), mb is one host-wide ~/.bashy/mb/posts.jsonl (ForReader is a view, not authorization), and meet/sprint/chat/kb/memory/handoff/feed share single pools per host.

Streamlined model to record:
- Circle = named audience boundary. Built-in default circles unify directly with the Cloudbox account member list: owner (absolute, innermost, 0 members), family, friend, team (or team:<name> when multiple teams/satellites exist), guest, and public. On the Cloudbox account member list the circle becomes a validated `circle` field (guest/family/friend/team). Checked in code 2026-11-09: today the list has one free-form `role` field that the UI fills with guest/family/friend/team but invites and scoped pairing also use for admin/member, there is no Bearer-scoped members API, and accepting an invitation does not add the reverse row. Cloudbox support ticket a99e88ac (private cloudbox) splits circle from the pairing role, adds /api/v1/members + members:* scopes and a reverse "who lists me" view, and asks the operator to decide mutuality; this design records the decision. Circles are siblings, not a trust ladder; the owner human belongs to all, a session acts in exactly one.
- owner circle is a complete private space, not just a vault: secrets/accounts/tokens/keys (yoke/pkg/secrets, yoke/pkg/accounts) AND private todos, kb pages, memory, notes, sprint items, mb notes-to-self, chat/ask history, handoffs (~/.bashy/<tool>/owner/, 0700). Never in a repo ring, never on any transport (not even between two Owned: true outposts). Visible only to the owner human at an interactive TTY. Secrets are usable by agents only by handle (secret://...), scrubbed via secrets.ScrubAgentEnv and masked via secrets.Redactor; plaintext reveal requires the interactive TTY human approve gate (approve_src.go).
- Physical directory partitioning over per-record filtering: host stores partition via yoke/pkg/scope.Resolve into ~/.bashy/<tool>/ (default circle, legacy in-place with zero migration), ~/.bashy/<tool>/owner/ (0700), and ~/.bashy/<tool>/circles/<circle>/ so kb/index.md, journal.jsonl, and .git never mix circles on disk. Repo stores (<repo>/docs/*) inherit the repo's circle (.bashy/circle or git remote mapping); public repo committed stores = public.
- Reuse yoke/pkg/accounts + yoke/pkg/fleet (no parallel inventory in circles.yaml): Cloudbox member circles (guest/family/friend/team), paired outposts, accounts, CLI profiles (AWS_PROFILE, DKS_PROFILE), and LLM endpoints resolve through existing accounts.Account (and attached satellites) + fleet.Catalog. ~/.config/bashy/circles.yaml (0600, never committed) stays a minimal local routing/override file (path/remote -> circle map, default circle, owner email).
- Activation & Ceiling Lock: --circle / BASHY_CIRCLE > committed .bashy/circle > ~/.config/bashy/circles.yaml > default (family). CRITICAL: spawned agents and non-TTY children are ceiling-locked to their inherited BASHY_CIRCLE (anchored to BASHY_EPISODE in coord/identity.go); an agent passing --circle <other> or mutating BASHY_CIRCLE fails closed unless approved at an interactive human TTY.
- Rule Allows(dataCircle, target, op): same circle allow; owner -> anything deny except use-by-handle in owner's own exec; cross-circle write/send/push/share/exec-with-credential = fail closed; explicit cross-circle sharing uses `bashy circle share <ref> --to <circle>` (human approve + scrub gate, no query-time tag-share DSL); publish to public runs script/scrub-gate.sh + per-circle deny patterns.
- Prior art to reconcile: story 101f7ec3b611 (human-lane S0 I14/I15 audience intersection); S513 invite-only sharing (d51b4726a0f7, ff5066e6ccc8); kb agent ring owner-only check (yoke/pkg/kb/store.go); meet roster organizer rules; role-mail AuthorizeByHolder; Sprint 606 master/satellite decoupling.

Public repo: placeholder org/team names only (acme), hosts by role.
Acceptance: doc merged, streamlined decisions recorded, story list aligned.
