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

## Rescope 2026-11-19 (operator: "rescope it and MVP in light of the new fleet")

The original six-story MVP was written before the fleet changes of
2026-11-19: one-command steerable sessions (`bashy <tool>`, bashy 73c7d24c +
zero-token E2E f151dfb9), the capped chat inbox (yoke f4a1c2f2), working GPT-6.2
(codex pin 0.171.0) and conductor-reviewed weave lanes across six vendors. That
night a conductor ran 4 sprints and 12 weave lanes in parallel by hand. What
blocked it is the new MVP; what never came up is deferred.

| observed 2026-11-19 | need |
|---|---|
| the conductor knew a run had finished only because its own harness notified it; a codex/agy/muse manager has no such signal | `sprint await` |
| steers sent while a worker was starting were lost (runs #113, #114 re-queued) | steer delivery that waits for the session to be ready |
| every gated bashy merge failed on 4 pre-existing failures (todo 66d98310) and was merged by hand with evidence | gates judge regressions against the base's known failures |
| the same operator rules were pasted into every brief (root cause first, no push, one run at a time, never push a cloudbox-v* tag) | `sprint directive` appended to every brief and manager payload |
| bad runs were abandoned (#68, #115, a go mod tidy that dropped a build dependency) with no record on the card | `sprint reject` with a recorded reason |
| `sprint show` took 2-4 min; `goal add --story` cannot link stories outside the umbrella; conductor todo edits tripped weave's isolation alarm three times | small fixes |
| taking a stale seat worked directly (673, 674, 676); nothing needed re-triage | defer cooperative take and retriage |

## MVP (rescoped)

| Pri | Story | Outcome |
|---|---|---|
| P0 | 48a92d65 `sprint await` | one blocking call returns the next typed actionable event (run submitted/failed, reply, stall) with a suggested verb; works for any vendor's manager |
| P0 | 6a1fabca steer delivery | a steer to a starting session is queued until its first prompt is ready (no lost steers); empty-verify submit refused |
| P0 | new: gate baseline | a weave suite gate fails only on failures absent from the base commit; pre-existing ones are reported, not blocking |
| P1 | 561bac1b `sprint directive` | standing operator rules and pre-approvals on the card, appended to every weave brief and manager payload |
| P1 | f8fed0a7 `sprint reject` | rejection recorded on the card and the run; self-accept guard deferred |
| P1 | new: manager friction | `sprint show` under 10 s; cross-repo `goal add --story`; conductor edits under docs/todo do not trip isolation |
| P1 | 6b7b112d playbook (bashy part) | text matches the loop above: lanes, `await`, review, `reject`, `directive`, `bashy <tool>` for operator-started sessions |

Deferred (post-MVP, off the sprint): 92cb9ce5 cooperative take, 1f642a92
retriage, the self-accept guard. Unchanged post-MVP: supervisor daemon,
`sprint attach`, attention queue, vendor-spread alerts, `sprint brief`.
Friction 407a3a92: its overflow-loop half is fixed (yoke f4a1c2f2, todo
5e01de0e); the rest (worker tests announce to the real board) joins 9ac774dd.

## Rules

- Extend existing code paths: `yoke/pkg/weave` (sprint, lease, notices, gates)
  and `bashy/internal/agentos`. No new transport or store.
- Each story ships with unit tests; Go gates `GOWORK=off`. Heavy gates on the
  self-hosted test hosts, never the dev box. bashy uses `bashy dag`, not make.
- Workers run their own targeted tests; the conductor reviews every diff and
  merges; workers never push.
- Public repo: no host names, logins, addresses or home paths in any file.

## Acceptance

All P0/P1 stories accepted with unit tests and the bashy E2E green. Effect is
measured on live sprints (673, 676), not Sprint 562: a manager reacts to a
finished run through `sprint await` with no harness notification; zero lost
steers in the E2E; a gated weave pull merges with pre-existing failures present
and blocks a newly introduced one; at least one `reject` and one `directive`
recorded on a card.
