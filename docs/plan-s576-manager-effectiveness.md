# Sprint 576 — proactive sprint manager (MVP)

## Problem

A sprint manager running in an operator-started session acts only when the
operator types. In the Sprint 562 assessment the manager started 0 of 69 turns
itself, reacted to finished worker results 4–9.5 hours late, left 18
submissions pending for 29–38 hours, never recorded a rejection, accepted five
of its own stories, and let 10 planned stories grow to 41 without a recorded
re-plan. Bashy already has most of the parts (lease, notices, `sprint wait`,
`take --force`, managed sessions via `sprint start`); what is missing is a small
set of verbs and instructions that make the manager react, review, re-plan and
hand over without operator nudges.

## Scope (six stories, keep it simple)

| Pri | Story | Outcome |
|---|---|---|
| P0 | 6b7b112d playbook | THE TICK step 5 says workers run gates; turn-end contract (dispatch or `sprint await`, never "will check later"); operator section in `skills/bashy/commands/sprint.md` |
| P0 | 92cb9ce5 cooperative takeover | `sprint take --force --approved-by OP --reason` notifies the incumbent, waits for its `handoff` up to `--grace`, then transfers; former holder refused |
| P0 | 48a92d65 `sprint await` | one blocking call returning the next typed actionable event with a suggested verb |
| P1 | f8fed0a7 reject + guard | `sprint reject`; `accept` refuses self-accept without audited override |
| P1 | 1f642a92 `sprint retriage` | untriaged intake marker; must/should/defer/drop; defer/drop need operator approval unless pre-approved; carry with `--to N` |
| P1 | 561bac1b `sprint directive` | standing operator rules and pre-approvals on the card, injected into manager payloads |

Out of scope (backlog, label `post-mvp`): supervisor daemon, `sprint attach`,
attention queue on show, vendor-spread alerts, manager-does-work detector,
note lint, goal prompts, `sprint brief`, inbox instance routing.

## Rules

- Extend existing code paths: `yoke/pkg/weave` (take/handoff/lease, claim/
  accept, goal/next) and `bashy/internal/agentos` (`sprint_wait.go`). No new
  transport or store; story state stays in `docs/todo/` frontmatter and the
  sprint thread.
- Each story ships with unit tests; gates run `GOWORK=off` on the self-hosted
  test hosts, never the dev box. Final gate: `make test && make test-bash`.
- Workers run their own gates; the manager reviews from bashy evidence.
- Public repo: no host names, logins, addresses or home paths in any file.

## Acceptance

All six stories accepted with their unit tests; `bashy` `make test` and the
serial `make test-bash` gate green on a frozen candidate with no regression from
the starting baseline; the playbook text renders in `sprint --help` and
`bashy skill show sprint`. Effect is measured on Sprint 562 supervision: a
manager-initiated reaction to a finished worker run, at least one recorded
`reject`, and at least one `retriage` decision with operator approval.
