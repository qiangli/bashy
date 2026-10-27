---
id: b31c9bdb4748
kind: doc
title: 'S514.R9 workplace rule in bashy skills: every seat watches its inbox, answers promptly, collaborates with other managers'
seq: 2737
status: todo
priority: p1
labels:
    - skills
created: 2026-10-27T17:15:53.295435Z
sprint: 514
sprint_id: fadd1a46-0368-50f1-a65d-4ed147b17629
sprint_title: 'invitation email + scoped satellites (KISS/MVP: scope + expiry on the one-time pairing code; no outpost change)'
---

Operator 2026-10-27: a general rule for all bashy workplace users, not only dhnt. Add short generic text (2-5 lines each; no dhnt names, no hostnames) to: skills/bashy/commands/inbox.md (whoever holds a seat keeps 'bashy inbox --as <seat> --watch' running or peeks every few minutes while working; answer promptly, blocking requests first: claims, shared hosts, shared checkouts), skills/bashy/commands/conductor.md and sprint.md (sprint managers collaborate via 'bashy mb send <seat>' and announce before taking a shared host, committing in a shared checkout where another lane has runs - prefer a scratch worktree - or bumping a pin another lane depends on; a held claim another lane asks for is released or explained at once). Regenerate any skill golden the tests check (internal/agentos/testdata/skillhelp/*). Gate: go test -short ./internal/agentos with a clean PATH. Coordinate with the unmerged S514.R7 (same files; rebase on main after it lands).
