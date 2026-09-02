# Sprint derived goal and priority-first execution

## Contract

- The sprint card owns durable outcomes, acceptance/gate evidence, execution
  policy, current focus, continuity, and owner identity.
- Repo todo stories remain authoritative for status and priority. Sprint views
  derive checklist completion and the next runnable story; they never copy an
  ordered backlog into sprint prose.
- P0 → P1 → P2 → P3 is mandatory. A lower-priority focus requires a recorded
  override reason.
- A sprint with checklist items cannot move to `done` while any item is
  unchecked. Story closure checks linked items; reopening unchecks them. A
  gate-required or storyless item also requires recorded evidence.
- A takeover keeps the recorded owner name as its coordination identity unless
  it explicitly changes ownership first. A managed session uses automatic inbox
  delivery; an external harness retains an exact-name `sprint ... --watch`
  foreground stream while managing the sprint.

## Delivery slices

- [x] Backward-compatible sprint storage for goal items, story roots, execution
  policy, focus, and evidence.
- [x] `sprint track`, `sprint goal add|link|evidence`, `sprint next`, and
  `sprint focus` command surface.
- [x] Derived checklist/next-story rendering and JSON progress.
- [x] Priority enforcement with an audited override and done-column refusal.
- [x] Take/resume identity preservation and verified managed-inbox delivery.
- [x] Project write lease already enforces commit/push/merge/rebase conflicts;
  add `bashy claim request` so a blocked agent asks the live owner to
  review/merge/sequence/release without stealing the lock.
- [x] Owner guidance covers end-to-end delivery, proactive integration and
  cleanup, cross-sprint coordination, and the absolute prohibition against
  deleting work owned by another sprint or agent.

## Gates

- Focused Coreutils tests for derived priority, closure/reopen behavior,
  evidence, takeover identity, sprint commands, and project claims.
- Bashy agentos tests for embedded help/skill contract.
- Full Coreutils package suite and cross-vet.
- Full Bashy Go suite plus six-platform lean cross-build gate.
- Dragon rebuild/install and smoke through the installed PATH binary.
