---
id: 92b67dc42fd0
kind: doc
title: 'S583.1 Boundary control design of record: circles (owner/personal/org/team/public)'
seq: 3132
status: todo
priority: p0
labels:
    - boundary
    - design
created: 2026-11-06T21:45:55.324575Z
sprint: 583
sprint_id: 2035b56b-d719-5d80-8c39-6195df0d1e6a
sprint_title: 'Boundary control: circles (owner / personal / org / team) across all of bashy'
---

Design of record for Sprint 583 boundary control. Deliver docs/boundary.md (abstract in docs/README.md if present, one line in docs/INDEX.md). Design only, no code.

Problem: a human knows their audience (when in Rome, do as the Romans do): family matters stay out of the office, coworkers do not see private life, account secrets are shown to nobody. bashy has none of this. Principal owner is hard-coded "local" (yoke/pkg/principal/principal.go), scope stops at repo|user|agent (yoke/pkg/scope/scope.go), mb is public by design (one ~/.bashy/mb/posts.jsonl per host; ForReader is a view, not authorization), meet/sprint/chat/kb host ring/memory/handoff/feed are single pools per host, credentials are scoped per tool only, and the only egress privacy check is the regex scrub gate on commits.

Model to record:
- Circle = named audience boundary. owner (absolute, innermost), personal circles (home, friends, ...), org:<name>[/team:<name>], public. Circles are siblings, not a trust ladder; the owner human belongs to all, a session acts in exactly one.
- owner circle is a complete private space, not just a vault: secrets/accounts/tokens/keys AND private todos, kb pages, memory, notes, sprint items, mb notes-to-self, chat/ask history, handoffs. Never in a repo ring, never on any transport. Visible only to the owner human at an interactive TTY - not family, friends, coworkers or agents. Secrets are usable by agents only by handle (secret://...), injected at exec, never revealed; reveal = human-only approve gate, TTY only.
- Labels: every datum carries circle: (frontmatter via issue.Issue.Extra / kb Page; JSON field elsewhere). Unlabeled legacy data = default circle (config, default home); public repo committed stores = public. No bulk rewrite.
- Members: humans, agents, fleet entries, peers, hosts, cloudbox sessions carry circle sets (fleet catalog org overlay ring maps to org:). Spawned agents inherit only the session circle via BASHY_CIRCLE.
- Activation order: --circle / BASHY_CIRCLE (audited override) > committed repo binding .bashy/circle > host-private ~/.config/bashy/circles.yaml (path/remote map, definitions, members, credential + LLM endpoint bindings; never committed) > default.
- Rule Allows(dataCircle, target, op): same circle allow; owner -> anything deny except use-by-handle in owner's own exec; cross-circle read/list = filtered out (owner-human-only TTY --all-circles escape, never agents); cross-circle write/send/push/share/exec-with-credential = fail closed unless an explicit share rule in circles.yaml; publish to public runs the scrub gate extended with per-circle deny patterns.
- Coverage table for every bashy surface (see stories S583.2-S583.14). Plain bash builtins and coreutils unchanged (frozen baseline / POSIX scope); enforcement lives in the agentic layer only.
- Enforcement points: new yoke/pkg/boundary (single source of truth, no import back into bashy), yoke/pkg/scope + principal, in-store filters (todo/issue, kb, bus board, meet session, weave sprint, chat, handoff, feed), bashy ExecHandler boundaryGuard after claimGuard (bashy/internal/agentos/agentos.go middleware chain), coordGuard git remote check, registry ExecEnv.

Prior art to reconcile, not supersede: story 101f7ec3b611 (human-lane S0 audience rules I14 declared relations / I15 intersection of present principals); S513 invite-only sharing stories (d51b4726a0f7, ff5066e6ccc8); kb agent ring owner-only check (yoke/pkg/kb/store.go); meet roster organizer rules; role-mail AuthorizeByHolder.

Open decisions the doc must close with the operator before S583.2 starts:
1. Agents in owner: default = per-session grant via human approve gate (secrets still handle-only, owner-bound local model endpoints only, output stays in owner); strict alternative = human-only, no agent ever reads owner todo/kb.
2. Activation: location-derived + explicit override (default) vs explicit switch only vs per-command flag only.
3. Enforcement: hard on egress and owner material, filter on read, owner-only --all-circles (default) vs fail-closed everywhere vs warn-only first stage.
4. Naming: "circle" (avoid "context" = inspect context, "cluster"/"sphere" = tier names).

Public repo: placeholder org names only (acme), hosts by role.
Acceptance: doc merged, decisions recorded with operator approval, story list below re-checked against the doc.
