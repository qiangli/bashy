---
name: sprint
description: Start or direct a Bashy sprint while requiring an explicit canonical project manager and managed delivery. Use when the user invokes /sprint or asks an agent to create, start, or direct a sprint.
---

# Sprint

Translate the user's request into Bashy sprint commands. This skill is the
agent-facing `/sprint` adapter; do not build or invoke a second prompt parser.

## Select a sprint

Every sprint follows the same lifecycle, authorization and acceptance rules.
No maintenance number or fixed maintenance UUID is reserved. Use a sprint
number for a specific card or `last` for the most recently created card.
Commands also accept `--sprint N` or `--sprint last` instead of the sprint
operand. Do not combine the flag and operand. Omitting selection uses the
same last card; it never creates one implicitly.
An empty board requires creating a sprint before selecting it.

Repository todos use the selected sprint: `todo add "fix something" --sprint N`
or `--sprint last`. Use `--no-sprint` for an unlinked todo. Personal todos remain
unlinked by default. `todo edit ID --sprint 0` unlinks an existing item.

Read the card and story, coordinate with its manager, and move backlog/done to
doing before claiming. Filing a story does not reopen a sprint or appoint a
manager. Normal story scope, evidence and commit trailers still apply.

```text
bashy sprint show last
bashy todo add "fix something" --sprint last --note "Problem, behavior, verification"
bashy sprint claim last STORY_ID --owner YOUR_REGISTERED_AGENT
bashy sprint submit last STORY_ID --as YOUR_REGISTERED_AGENT -m "delivery evidence"
bashy sprint accept last STORY_ID -m "manager verification"
```

## Resolve the manager

- Never choose a default manager or guess an identity.
- If the user gave an exact manager, verify it with `bashy agent list`.
- If the manager is missing or ambiguous, inspect `bashy agent list`, present
  the relevant canonical names, and ask the user to choose before mutating the
  sprint.

## Start a new sprint

Create the card with `bashy sprint add "Title"` (or add `--json` for structured
output). Bashy allocates and returns the next available number; do not supply
a number or a `next` selector. Use that returned ID for the plan and lifecycle
commands below. Sprint 9999 is historical and has no special maintenance role.

Before taking or starting the seat, inspect the affected code, read every linked
story, and write or update the master execution plan. Record both orientation
facts on the card; Bashy refuses `take` and `start` when either is absent:

Before assigning work to a shared test host, droplet, device, or account, take
its host-local advisory hold with `bashy claim NAME --intent TEXT`; release it
when done and never force another holder off without contacting them.

For workplace coordination, follow `bashy skill show collaborate`.

```text
bashy sprint edit ID --primary-goal "ONE OUTCOME" --spec docs/sprint-ID-master-execution-plan.md
```

Then run:

```text
bashy sprint start ID --owner NAME --instruction TEXT
```

Pass the complete user instruction as one argument, preserving its bytes. Do
not interpolate it into shell source. A successful response must name the
sprint, owner, managed session, and Meet contact.

## Direct an active sprint

Inspect it with `bashy sprint show ID`. Preserve its current owner, then run:

```text
bashy sprint instruct ID --instruction TEXT
```

Do not supply or change an owner on this path. The command must reuse the
current owner's managed session.

Sprint stories use one fail-closed path: `sprint claim`, then `sprint submit`
with delivery evidence, then the current manager runs `sprint accept` with the
verification it independently checked. Generic `todo done` never closes a
sprint story.

## Claim, workspace, and grade retention lifecycle

- **Announce vs lease:** `sprint claim` records an `announce` claim
  (`story:<sprint>/<id>`, no TTL) alongside the story's `assignee`; resource and
  project holds (`bashy claim NAME [--story SPRINT/STORY]`) use TTL-backed
  `lease` mode (or child-scoped `attached` mode with `-- cmd`). Workers launched
  via `weave start` export `WEAVE_STORY` so any `bashy claim` inside the run
  is linked to that story automatically.
- **Delete by default (`--retain` opt-in):** `sprint grade` deletes its temporary
  checkout immediately after recording the grade event unless
  `--retain --retain-reason "<why>"` is passed. Weave terminal transitions
  (`weave pull`, `weave abandon`, and empty `no-op` exits) likewise delete run
  workspaces by default unless `--retain --retain-reason "<why>"` is set.
- **Close reclaims everything:** `sprint accept`, `todo done`, `wontfix`, moving
  a story off a sprint (`todo edit ID --sprint 0|OTHER`), and `sprint end` all
  call one guarded reclaim path: they release every `story` claim on the story
  (any holder) plus every lease/project claim naming that story, end `--retain`
  retention, delete settled weave workspaces and grade checkouts, and append
  `reclaim` audit events on `bashy events` with the closing verb as reason.
  Workspaces holding dirty files or unique commits not in the base branch or
  `refs/salvage/*` are **refused, never deleted**.
- **Backfill:** `bashy claim prune --stories-closed` (dry-run default; `--apply`
  to execute) clears stale claims and settled run/grade copies left behind by
  already-closed or provably missing stories.

Dispatch runs in lanes: `bashy sprint assign` starts one weave run per story in
its own isolated workspace, and the manager reviews each submission — accept,
or `bashy sprint reject ID STORY -m "feedback"`, which records the decision and
returns the story to doing with its assignee. For an operator-started,
steerable session outside the sprint lanes, `bashy <tool>` opens one directly;
it joins the same room, so `bashy chat sessions` and `bashy chat steer` reach it.

Turn-end contract: every manager turn ends with a dispatched action or a
blocking `bashy sprint await` — one typed event (submission, run
submitted/failed/stalled, manager mail) with a suggested verb — never a
promise to check later.

## Operator quick map

| Operator intent | Command |
|---|---|
| Status | `bashy sprint show ID` |
| Nudge / redirect | `bashy sprint instruct ID --instruction TEXT` |
| Standing rule or pre-approval | `bashy sprint directive add ID "RULE" [--preapprove RULE]` |
| Change scope | `bashy sprint edit ID …` plus `bashy todo add/edit --sprint ID` (unlink with `--sprint 0`); `sprint retriage` is deferred and does not exist yet |
| Take over the seat | `bashy sprint take ID --owner NAME --force --watch`, then record the operator approval as a sprint comment (`bashy sprint comment ID "…" --kind decision`) |

Directives are appended to every manager payload and weave brief, so a rule
recorded once reaches every lane.

Stop and report the command error if launch or instruction delivery fails. Do
not claim that work was dispatched unless Bashy confirms it.

For manager fast-loop practices (native repro of platform failures, hermetic tests,
parallel pre-flight, producer-before-consumer pin bumps) see the conductor skill.
