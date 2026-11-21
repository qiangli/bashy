---
id: e868c8201747
kind: feature
title: 'Point-based LLM budgets: story/sprint/band envelopes from Fibonacci points, calibrated per project'
seq: 3284
status: todo
priority: p2
labels:
    - budget
created: 2026-11-21T10:28:35.176524Z
sprint: 688
sprint_id: b6d0bf82-bf8a-5ba7-bbab-90b24be56deb
sprint_title: 'Worker supervision round 2: session state, rate-limit handoff, spend watermarks, board performance'
---

Operator request 2026-11-21 (quota-exhaustion review, umbrella docs/sprint-688-master-execution-plan.md §2a). General bashy feature for every user, not one fleet.

Already exists (reuse, no new store): Fibonacci story points 1/2/3/5/8 (yoke weave_impl.go Points, sprint_estimate.go), llmbudget constraints with daily/weekly tokens AND daily/weekly_spend_micro_usd, Reserve/Settle with stable operation IDs, OwnerLease, manager bands (sprint_band_gate.go).
Gap: constraint scopes are provider/account/pool/lane/host only; nothing ties spend to a story, a sprint or a band.

Shape (smallest slice):
1. Envelope = points x rate. Story envelope = points x per-point rate; sprint envelope = sum of its stories' envelopes unless the manager sets an explicit cap; band = APPROVAL AUTHORITY only, not spending allowance (operator decision 2026-11-21): budget follows the work, so a story costs the same whoever works it and routing to a higher band never buys more budget. An operator-set table gives each band the overrun it may approve for others (L4 manager absorbs small worker overruns; larger ones escalate to L5/operator); promotion/demotion changes it. The sprint envelope adds one manager-overhead line for orchestration. Per-agent runaway protection stays the existing per-account caps.
2. Rate per point is calibrated per project (repo): rolling median of SETTLED actual usage per point on accepted stories. Only trustworthy sources count: API-lane settled cost/tokens; subscription lanes use measured window-% delta where a source exists, else tool calls (weave_story.go already warns the per-call cost meter is unreliable for harness-estimated tokens). Start from a conservative user-configured default; show sample count + confidence.
3. Enforcement: Reserve carries story/sprint/band scope. Soft warning at 80% (checkpoint + STATUS note, same path as watermark alerts); at 100% metered lanes are refused -> deterministic handoff (26bbbf01) and the manager decides: re-estimate, split, or approve overrun with a logged override (every guard has an escape hatch). Subscription lanes: soft only (no money spent).
4. Report: estimate vs actual per story/sprint/band in sprint monitor and at sprint end (feeds calibration).

Non-goals: new daemon/store, guessed prices, cross-host global quota.
Tests: envelope math; calibration from fixture settlements (median, min-samples fallback); soft/hard crossing once per story; override logged; subscription lanes never hard-blocked.
Depends on: 49cd45d9 (watermarks/alerts), 26bbbf01 (handoff).

## Addition (operator, 2026-11-21): minimum band per story

Time/budget and the band a task needs are correlated, so a story carries a
minimum band next to its points.

Today (source-verified): a story has a band (`sprint assign --band 3|4|5`,
`ladder.StoryTask.Band`), but it is a TARGET. When no agent at that band is
free, `ladder.ScheduleStory` drops band by band (`cascade:L<n>`,
`yoke/pkg/ladder/scheduler.go`) and `sprint assign` only records a `fallback`
event. Entrants already carry `CostPerPoint`.

Shape:
1. Estimation records both values: points (size/effort) and min band
   (difficulty/judgment). They are related but distinct. The suggested band
   defaults from points through a calibrated per-project table; the manager
   can raise or lower it, and the reason is recorded.
2. The floor is hard. The cascade stops at the story's min band; below it the
   pick is `wait` with the reason "no free agent at >= L<n>". The escape hatch
   is the existing manual `--agent` override, logged in the thread with the
   approver, whose band must be >= the floor. A story waiting on its floor
   shows on the board/monitor, so the manager can re-band, split or wait.
3. The budget envelope = points x rate(project, band). The rate is calibrated
   per band from settled usage on accepted stories (and entrant
   `CostPerPoint`), so the band/budget correlation is measured, not assumed.
4. Calibration report at sprint end: points, band, actual usage and outcome
   (accepted/reworked) per story. This shows whether a band was set too low
   (rework, overruns) or too high (an expensive agent on an easy story), and
   feeds the default table.

Tests: the cascade never goes below the floor; a manual override below the
floor is refused unless approved and logged; envelope math per band;
calibration from fixtures.

## Status: experiment (operator, 2026-11-21), review around 2027-02-21

Budget and min-band management of sprints and stories is an experiment. Run it
for a few months, then keep, tune or remove it based on evidence.
- Opt-in per sprint (a sprint-level switch), off by default for other users
  until the review. Enforcement stays observe-only (report what would have
  been refused or waited) until the manager turns it on for a sprint.
- Record per story from day one: points, min band, assigned band, estimated
  envelope, actual usage, waits on the floor, overrides, outcome (accepted or
  reworked).
- Review questions: do estimates converge (actual/estimate per band)? Did the
  floor cut rework and wasted tokens on hard stories? How much waiting did it
  cause? Were overrides frequent (floor or table wrong)?
- Removal must be cheap: no change to the story schema beyond optional fields,
  and no impact on sprints that never opted in.

## Scope decision (operator, 2026-11-21): build it, do not enforce

This story ships OBSERVE-ONLY. It supersedes the enforcement items above (the
80%/100% refusal, the hard min-band floor, the sprint enable switch):
- Compute and record per story: envelope (points x rate(project, band)), min
  band, assigned band, actual usage, outcome.
- Where enforcement WOULD have acted, record a `would-refuse` / `would-wait`
  event (budget crossing; assignment below the floor) on the story thread and
  in the sprint-end report. Never block, refuse, delay or reroute work.
- On by default for recording, since it is passive and costs no model calls.
- Enforcement (refusal, hard floor, override approvals) is decided at the
  2027-02-21 review from this evidence; no code for it now.
Tests: envelope math; calibration from fixtures; would-refuse/would-wait
events emitted once per crossing and never changing the pick or launch.
