---
name: conductor
description: >-
  Conduct a fleet of agentic coding CLIs (claude, codex, opencode, ycode, agy) to
  reach a VERIFIED goal — decompose → isolate → gate → converge, looping until a
  verifier passes — acting as the single conductor over `bashy sprint`
  (plan/continuity) and `bashy weave` (per-repo isolated execution). Use when a
  goal decomposes into many independent, gateable units: a deterministic failing
  test suite (the canonical case), a port/migration, a broad mechanical refactor,
  a conformance push, a coverage drive — with a fast repro and a regression guard
  to protect. NOT for a one- or two-edit fix (do it inline) or a goal with no
  way to verify "done" (define a gate, or stabilize a flaky suite, first).
---

# conductor — drive a fleet of agent CLIs to a verified goal

You are the **conductor**. You never write the fix. You read the goal, set the
contract, decompose, file stories, build the isolation + gates, launch and
monitor the fleet, salvage killed runs, gate every merge, and iterate. The agents
do all analysis and code; you own the *loop* and the *truth*.

**One principle governs everything: the queue and the measured numbers are the
only truth.** A worker's prose or commit message is a lead until the gate
reproduces it. Echo measured numbers verbatim; never accept "submitted" as
"done" — re-measure against the goal.

**Run the gate YOURSELF, and read the verdict from bashy — never from the
worker's output.** The primitive is `bashy gate --command '<gate>' --json`: it
*runs* the gate and emits a `bashy-gate-v1` verdict (`"passed": true/false`),
which a worker cannot fake. Do NOT grep the agent's stdout/log for the result —
a worker echoes your brief, and your brief names the success string, so its log
will contain `[gate] PASS` (or your equivalent) as *text the agent quoted*, not
as a gate that ran. That exact false-positive fires a "converged" on a worker
that changed nothing. The verdict must come from a command you ran, not a string
you found. (This is the runtime face of the fleet-evidence-invariant: a success
state must be reached by evidence you produced, never by the absence — or the
echo — of the worker's.)

This file is the actionable checklist. The full narrative — the four isolation
traps in depth, convergence details, and two worked campaigns — lives in the
bundled `reference.md`; read it before your first campaign.

## Seat authority — you are APPOINTED, you are not self-selected

**The steward appoints and qualifies conductor seats.** A conductor never selects
its own seat, never qualifies itself, and never names its own successor. If no
steward has appointed you to this workstream, you do not hold the seat — ask the
steward for the appointment rather than assuming it.

What you *do* own, exclusively, is the **worker fleet**: how many workers your
assignment needs, which tool/model runs each story, how they are isolated,
steered, interrupted, reassigned, gated, and merged back. The steward does not
size, select, or steer your workers; you do not size or select conductor seats.

**One agent must not hold a steward seat and a conductor seat at the same time.**
The steward's job is to judge conductor outcomes by independent evidence; an
agent that appoints, drives, and reviews itself has no independent layer left and
every check collapses into self-report. If you are already the host steward and a
conductor seat is needed, appoint a *different* agent to it.

Three rules follow, and they are not negotiable:

- **Sub-hubs stay inside your ownership.** If you appoint a foreman or any
  sub-hub, it leads a *sub-team of your workers* under your contract and your
  merge gate. It creates **no steward-facing ownership layer**: the steward keeps
  addressing you, the appointed conductor, and never the foreman or its workers.
  You remain accountable for everything the sub-hub does.
- **Standby failover is conductor-internal, and it checkpoints.** You may run a
  cold spare inside your own workstream and let it take the lease (fencing-epoch
  protected) if you go dark. Activation **checkpoints and notifies the steward**
  — it is a continuity event the steward must see, not a private swap. It never
  implies a lateral transfer of authority: the standby inherits *your* appointed
  scope unchanged, and nothing more.
- **Scope moves only through the steward.** You may *request* release of scope
  you should not hold, or transfer of scope you need. You **cannot hand scope
  laterally to another conductor, and cannot accept scope handed to you by one.**
  The steward records the old and new boundary **before** the receiving conductor
  acts; until that record exists, the work is still the original owner's.

## The goal is the contract

A conductor run is **done** iff three checks hold — the *same* contract for every
tool, so a strong model, a weak model, and a deterministic runner are all judged
identically and results converge regardless of who did the work:

1. **goal-met** — the goal verifier exits 0. The verifier is whatever proves the
   goal: `go test ./...` / a target test suite green / `make`, a linter, a custom
   gate, or (default) "all queued work merged". For goals with no exit-coded
   check ("the docs read clearly", "the refactor is simpler"), use **judge mode**
   (below) — a model verdict, defaulting to "not met" when unsure.
2. **converged** — no open/unmerged work remains in the queue.
3. **reviewed** — an *independent* post-convergence check passes (a regression
   gate, so a merged combination that breaks the tree is caught before accept;
   defaults to re-running the goal verifier on the merged tree).

"Until done" means you re-invoke the conductor until all three pass — there is no
loop construct, the contract *is* the loop condition. An unmet goal is **not a
crash**: surface the failing checks as **blockers**, record them in the
continuity note, and exit cleanly. Each run makes progress; you re-invoke to
continue.

## Effect cap (blast radius)

A conductor run may **read, write, reach the network, spend tokens, and burn
wall-clock** — and **must not destroy**: no `git add -A`, no `rm -rf`, no force
push, no dependency-pin/submodule bump without explicit human OK. The fleet
writes inside isolated workspaces; convergence is a sequential, gated, reviewable
merge. Keep the blast radius bounded by construction, not vigilance.

## Preconditions (verify before starting)

1. A goal with a **measurable acceptance** you can name, run, and score
   (per-case pass/fail for a suite; an exit-coded gate otherwise).
2. A **fast single-unit repro** against the built artifact, runnable in a
   workspace with no heavy infra. If absent, BUILD IT FIRST — it is the biggest
   multiplier on agent throughput.
3. A **regression guard** — what must not break (the same suite's passing cases,
   a second suite, or a smoke gate).
4. Enough independent units that decomposition beats inline fixing.

## Hierarchy (the vocabulary)

**Campaign → Sprint → Story → Run**

- **Campaign** — the durable goal; acceptance is one measured predicate; spans
  many sprints; carries a continuity record + a single conductor lease. Anchor:
  `bashy sprint` epic + baton.
- **Sprint** — one bounded gated pass (backlog → fleet → gate → converge), each
  re-baselined on the prior's merged result. Anchor: a `bashy sprint` card.
- **Story** — one independently-deliverable, root-cause-coherent unit with its
  own target acceptance + verify gate; scope-disjoint from siblings. Anchor: a
  `bashy weave` issue.
- **Run** — one execution of a story by one tool in one isolated workspace; a
  story may take several runs (reassign / re-drive / salvage). Anchor: a `bashy
  weave start`.

Invariant at every level: an *acceptance* (measured predicate) + a *gate* (the
command that decides it); done only on a reproduced measurement; convergence
always ends with a **re-measure on the merged tree**.

## The phase loop

PLAN → (RESEARCH) → FAN-OUT → STEER → CONVERGE → RETRO. Drive it by hand with
`bashy weave` / `bashy sprint`:

1. **PLAN** — **check the host kb first: `bashy kb search <goal terms>`** —
   the collective memory of every agent on this host across all repos; known
   traps it returns go into story bodies as KNOWN TRAPS, and if nothing
   relevant exists note what you'd expect to find (you'll contribute it at
   RETRO). Then decompose the goal into disjoint-scope stories, file them in
   the queue (`bashy weave add … --priority p0`). Optional cheap-agent
   estimates. (Workers get their own kb check for free: `weave start` drops
   KB.md into each workspace.)
2. **RESEARCH** *(only when complex)* — if the queue is large, research
   approaches / prior-art / risks first. Simple goals skip it.
3. **FAN-OUT** *(routed by parallel-safety — see Scheduling)* — fan out to a
   **fleet** (one agent per story, isolated workspaces) **only when the work is
   many AND disjoint**; a single story or shared-source work routes to
   **sequential** (one worker grinding + resuming).
   - **Pre-flight the fleet's AUTH-readiness first: `bashy weave fleet --auth`.**
     A tool can be installed and "available" on PATH yet **not signed in** — it
     then stalls its worker silently until the idle-timeout (the agy-sign-in
     trap). `--auth` live-probes each tool and reports `ready` / `needs-login`
     (with a hint) / `stale-contract`; **drop any `needs-login` tool from this
     round** (or sign it in) rather than dispatching it into a stall. Launch each
     survivor with the explicit headless form — never `--tool` bare, which hangs
     at the trust prompt: `weave start --issue N -- bash -c '<headless> "$WEAVE_ISSUE_BODY"'`.
4. **STEER** — watch and unblock, proactively: `bashy weave list`, `… log N`,
   and let the **gate broker** auto-clear interactive blocks (`weave wait --issue
   N --broker` classifies a live gate and routes trust→keystroke / OAuth→`browser
   login` / else→human); inject keystrokes manually with `bashy weave say N
   "<msg>"` only for non-gate questions. Judge each worker against
   the GOAL, not its state — a `submitted`/exited worker has often done only part
   (headless tools especially exit after a couple of easy fixes, often
   uncommitted). Re-measure before trusting "submitted"; resume with an
   explicitly-iterative prompt (measure → fix next cluster → gate → commit →
   repeat) until the goal holds or each remainder is a documented blocker.
5. **CONVERGE** — wait, then merge **verified** work back: `bashy weave wait`
   then `bashy weave pull`; re-run the goal verifier on the merged tree by hand
   before trusting it.
6. **RETRO** — capture the tool report card (which CLI did well on what) + any
   lessons; embed bisect findings into the next round's story bodies. **Close
   the kb loop: `bashy kb retro <terms>`** — validate pages that proved out
   (`bashy kb validate <slug> --evidence "<gate cmd/commit>"`), supersede
   what proved wrong, add the campaign's durable lessons (distilled strategy
   + failures-as-guardrails, never transcripts; NOOP when nothing durable).
   This is what makes the conductor — and every other agent on the host —
   improve across runs, not just within one.

## Conductor faculties — decide for yourself

You are the superman of the team: nothing above you dictates *how*. Reach for
these on your own judgement, per the task in hand — they are conductor
responsibilities, not separate skills to defer to:

- **Research when the task needs knowledge you lack.** Before decomposing an
  unfamiliar goal, run a research pass yourself: file a research story and assign
  it to a web-capable fleet member (a model CLI with browsing), or fetch the
  sources you need, then fold the findings (prior-art, API shapes, risks) into
  each story body. Skip it for goals you already understand — research is a
  branch, not a tax.

- **Autopilot for long-running campaigns.** For a campaign that outlasts one
  sitting, drive it unattended with `bashy weave autopilot` — it auto-dispatches
  the queue to the qualified fleet and re-drives stalled stories; `bashy weave
  autopilot --standby` runs a cold spare that takes the lease (fencing-epoch
  protected) if the active conductor goes dark — a conductor-internal spare, so
  **checkpoint and notify the steward on activation** and carry the appointed
  scope across unchanged (§Seat authority). To span idle gaps, **self-wake
  with `bashy schedule`** — schedule your own re-entry carrying the next
  instruction, e.g. `bashy schedule add --every 30m --prompt "re-drive stalled
  stories, then converge" -- bashy weave autopilot` (the prompt arrives as
  `BASHY_SCHEDULE_PROMPT`). That makes the loop span days without a human in the
  seat. Use `command time --budget <dur> --todo "<next step>"` to wrap a step
  whose overrun should leave you a TODO rather than stall silently.

- **Be the foreman — or appoint one.** For a single coherent sub-goal you lead
  the fleet directly. For a large or multi-front campaign, interview the pool
  (§Staffing) and **appoint a strong, qualified tool as a foreman**: hand it a
  scoped sub-goal, its own sub-queue + gate, and the context to lead a *sub-team*
  of agents, then have it report convergence back to you. This is hub-and-spoke
  with a sub-hub — you keep the campaign contract and the authoritative merge
  gate; the foreman owns its sub-loop. Delegate leadership when the fan-out is
  wider than one driver can steer — never delegate the merge gate. **A foreman
  you appoint is still your worker**: it lives entirely inside your exclusive
  worker ownership, adds no steward-facing ownership layer, and the steward keeps
  addressing you rather than it (§Seat authority).

## Scheduling strategy

Maximize velocity per token, not just "run agents in parallel":

1. **Route by parallel-safety, not scale.** Fleet *only when* tasks are many AND
   disjoint (non-overlapping source). A single task, or any set sharing an
   implementation, runs **sequentially**. Parallel agents on shared code each
   rewrite the same functions differently and **collide irreconcilably at merge**
   — costing more than one agent doing it in sequence. A flip-in-isolation is not
   a flip-when-integrated.
2. **Assign by capability.** Strongest-fit tool per story (the RETRO report
   card): deep multi-file → strongest model; tightly-pinned surgical edit →
   one-shot tool; verification/judging → a separate reviewer. Pick the *cheapest
   qualified* tool that clears the story's difficulty; cascade up on
   stuck/regression.
3. **Hard single-feature task = sequential grind with resume.** Decompose into
   bite-size, commit each reduction, resume until done (e.g. 143→32→10→0). Agents
   often hit the watchdog mid-work with an *uncommitted* fix — recover it and
   resume; don't discard.
4. **Race, don't merge, competing attempts.** To explore approaches to one hard
   problem, run agents in separate workspaces and take the single **furthest**
   result — never merge two independent attempts at the same feature.
5. **Gate every merge on the FULL guard, not the per-task measure.** A task can
   pass its own gate while breaking a sibling that shares code (CONVERGE/REVIEW
   exist for exactly this).

## Staffing — qualify YOUR worker fleet (before the loop)

Staffing here is **workers only** — the conductor seat itself is the steward's
appointment, not yours to pick or trial (§Seat authority). Staff **objectively**
(don't guess who's good) under a standing **human override** (any human may
pin/force/exclude any choice).

Hold the single-driver locks for the scope you were appointed to (`bashy weave
baton take --as <you>` / `bashy sprint take`), and re-write the baton/continuity
after every action so a standby activation or a steward-recorded transfer resumes
cleanly.

- **The fleet** — a pool qualified for **capability**, not pre-assigned a title.
  Per-story roles (coder / tester / reviewer / release / …) are the conductor's
  *runtime* decision: story → role → cheapest-capable tool. Qualify the pool with
  cheap gates first, stopping as soon as a tool is disqualified:
  1. **Assignable now** — `bashy weave fleet` (on PATH and not cooling down).
  2. **Launch contract valid** — `bashy weave fleet interview --live` (catches a
     CLI that renamed/drifted flags).
  3. **Smoke test** — one trivial prompt ("reply with exactly: OK"); a tool that
     emits nothing or instant-exits is dead weight.
  4. **Capability rating** — the carry-forward report card + prior-sprint
     outcomes; rates *what a tool can do*, not a title.
- **Going dark — checkpoint, then tell the steward.** Track your own budget;
  provider APIs don't expose remaining-quota, so frequent checkpoints are the
  only mitigation. At a stable point before you run out, `bashy sprint checkpoint`
  and **notify the steward** so it can decide the seat — activating your
  conductor-internal standby is a continuity event you report, not a successor you
  appoint. A forced `weave take --force` bumps the fencing epoch so a revived old
  conductor can't double-drive.

## The loop (operational)

### 1. Read the harness
Establish the **scoring contract** (how one unit reports pass/fail), the **fast
single-unit repro**, and **which artifact** the canonical scoreboard measures
(build + gate that same one, same env/filters/skips). For a suite, name the two
roles: *target* (turn green) and *guard* (keep green).

### 2. Measure the baseline
Record the **actionable failing/unmet set** (units + one-liners) and the
**passing count** (guard anchor). **Filter out non-actionable** items
(environment-specific, flaky, upstream-default, platform-ceiling) — often by
differencing against a reference oracle. Note **environment-divergent** units so
a run's local gate doesn't false-pass; re-verify them canonically at the end.

### 3. Group by root cause (not raw count)
Cluster by the code path that fixes them. A big cluster with ONE root cause stays
ONE story (don't shard a fix). A grab-bag of singles groups by sub-mechanism.
Size each story to ~30 min of agent work; split bigger ones. Note clusters that
share a source file — they parallelize but **merge sequentially** (§9).

### 4. Sprint + stories
```sh
bashy sprint add "<goal>" --acceptance "<target green AND guard green>" --column doing --epic <name>
bashy sprint take <id> --as conductor ;  bashy sprint checkpoint <id> --continuity "<baseline+plan>"
bashy weave add "<story>" --priority p0 --points 8 --tool <tool> --verify "$(cat gate.sh)" --body "$(cat story.md)"
bashy sprint link <id> --repo <repo> --task <issue>
bashy weave baton take --as <you>        # single-driver lock; re-write it after every action
```
**Story body** (in order): SETUP (workspace + isolation rules) · single-unit repro
· GOAL with exact unit ids + one-line each · root-cause hypothesis (file) · SCOPE
(disjoint dir allowlist) · GATE · commit discipline (named files, never `git add
-A`) · blockers escape (commit partial + `<TOPIC>-BLOCKERS.md` after 3 tries).
**Specificity drives yield** — paste exact units + the repro; embed prior bisect
findings so the next agent skips the trap.

### 5. Isolation (build BEFORE launching — four traps)
1. **Shared mutable dependency** the build resolves through a path weave shares
   across workspaces (a `replace => ../dep`, a submodule, a symlink): give each
   workspace a **private copy**, repoint the build via the manifest's redirect,
   and `git update-index --skip-worktree <manifest>` so the branch keeps the
   shared path while the build uses the private copy; exclude the private dir.
2. **Gitignored test data** isn't in the clone — **copy** it per workspace (copy,
   not symlink; in-tree-writing runners race a shared dir).
3. **RED baseline**: a clean workspace scoring below canonical (missing
   artifacts/locale/helpers) — gate on "no NEW failure beyond the known baseline"
   OR make the workspace env canonical (stub the missing pieces, guarded as a
   no-op in the real tree). Adopt the agent's stub-fix if it produces one.
4. **Sandbox scratch pollution**: tools write caches/litter; commit **named
   source paths** only, exclude the scratch dirs.

### 6. The verify gate (`--verify`, three clauses)
```
<build the artifact>       || exit 2          # 1. it builds
<run each target unit>     ; assert all PASS  # 2. the goal is met
<run the regression guard> ; assert no NEW failure beyond the known baseline  # 3. nothing broke
```
Clause 3 is **non-negotiable**: a gate of only "new units pass" misses guard
regressions, and broad changes routinely close targets while nicking a passer.
The conductor STILL re-runs the guard in the real repo post-merge.

### 7. Launch
Match tools to stories by the report card, hardest to strongest. Pre-seed each
tool's trust/permission cache, set watchdogs, background each:
```sh
bashy weave start --resume --issue N --max-runtime 45m --mem-limit 12g -- <tool> <recipe> "<body>" &
```
**Smoke-test every tool on a trivial prompt first** — assume some of the fleet is
dead weight.

> ### THE EXIT CODE IS NOT EVIDENCE. RUN THE GATE.
>
> Measured in a three-harness A/B (one model, one task, one gate): **all three
> harnesses exited 0 when they failed.** One had no write permission and produced
> nothing — exit 0. One never read the spec and wrote wrong code — exit 0. Every
> passing run — exit 0. **Three harnesses, two catastrophic failures, zero non-zero
> exits.**
>
> A conductor that trusts `$?` merges both. Never mark a story done on an exit code.
> The gate is the only thing that knows.

**Report card** (carry-forward; update with evidence, not with impressions):

| tool | as a WORKER | as a CONDUCTOR |
|---|---|---|
| **codex** | workhorse — honest, fast, best default | not measured |
| **claude** | strongest deep/multi-file; often hits the cap mid-fix (salvage it) | steward-grade (L4) |
| **opencode** | fastest, leanest diff in the A/B; historically can no-op with exit 0 — **verify the diff** | **good.** Decomposed a 7-gate goal into 6 issues unprompted, staffed 4 workers in parallel, self-recovered from a failed start. Repeat ratio **1.2×**. Weakness: it *stops* (goes idle) rather than pushing through — drive it. |
| **ycode** | first-party; slowest, writes the most code | not measured. Its edge is the **event channel** — it reports `turn.start`/`tool.call`/`turn.end`, so a turn's end is a fact rather than a silence you interpret. |
| **agy** | fine coder once corrected | **NO. Do not ask it to lead.** Measured: **377 tool calls, 40 distinct (9.4× repeat)** — read one file 26 times, looped forty minutes, produced no plan, never recovered. Demoted to L2. |
| **aider** | ~~—~~ | **RETIRED from the fleet.** It only sees files explicitly added to the chat, so it cannot discover the files a task refers to. A conductor hands out a TASK, not a file list. Still works if a human names it: `bashy invoke --agent aider:deepseek-v4-pro`. |

**The loop metric is the cheapest conductor health check you have:** total tool calls
÷ distinct tool calls. Above ~3× and the agent is grinding, not converging. Break it
with `bashy foreman interrupt <id>` (ESC) — a queued message will NOT reach an agent
stuck in a loop, because it only reads its queue between turns and that turn is never
going to end.

### 8. Monitor — event-driven, actively
One backgrounded wait per story; act on every wake (NOT host `sleep` loops):
```sh
bashy weave wait --issue N --timeout 50m &
```
- **submitted/killed** → measure against the GOAL; "submitted" ≠ done.
- **salvage watchdog kills** — uncommitted diffs are usually real progress;
  build, measure, commit named files, gate.
- **reassign / work-steal** — finished/dead agent gets the next story; a no-op
  story gets re-driven with a sharper prompt or a different tool.
- **blocked on a gate** → the **gate broker** handles it: run `bashy weave wait
  --issue N --broker` and it classifies the live PTY block *by type* and routes it
  automatically — trust prompt → keystroke (`weave say`), browser-OAuth →
  `bashy browser login <url>`, device-code/api-key/unresolvable → escalates to you
  through the foreman. You only answer what the broker escalates. (Manual fallback
  — a normal clarifying question, not an auth gate — is still `weave say N "<msg>"`
  after confirming the block is live, a child in a TTY wait, not stale scrollback.)
- NEVER measure the suite on the host while agents compile — load makes per-test
  timeouts flake into phantom regressions.

### 9. Converge — sequential gated merge on a review branch
No push / no dependency-pin bump without explicit human OK.
1. Source-only patch from each run's private workspace (`git diff base..HEAD --
   <source-dirs>`; never scratch).
2. `git apply --3way`; shared-file stories merge cleanly when sequential —
   resolve conflicts by **combining** fix-sets.
3. Apply non-dependency commits by named path; leave unrelated edits alone.
4. **Re-gate**: rebuild against the merged dependency, run the **guard in the
   real repo**, re-measure every merged target. Watch for **cross-cluster
   ripple** (stories that each pass in isolation can drop a case via interaction)
   — bisect any guard regression against the pre-merge commit.

### 10. Iterate
Re-run the goal against the **merged** branch to get the shrinking remainder;
re-divide and re-sprint on the cleaner base (workspaces now clone the merged
state → true-green baseline; embed each round's bisect findings into the next
round's bodies). Repeat until the actionable set is empty; verify
environment-divergent units + ripple canonically in a final pass.

## Bounding & judge mode

- **Bounded, not open-ended.** Cap rounds (stop the moment the contract holds)
  and stop cleanly if a spend probe reports over-budget (delegate measurement to
  `bashy weave cost --total` or your real cost source). Over-budget is a stop
  condition, not a contract failure.
- **Judge mode** (goals with no exit-coded verifier): gather evidence (a summary
  of the merged work + recent history) and ask an agent CLI whether the goal is
  *fully* achieved, defaulting to "not met" when unsure. The convergence gate
  stays deterministic; only the goal-met clause becomes a model verdict.

## Anti-patterns (each costs a round)
- `sleep`-loop monitoring instead of `bashy weave wait`.
- A gate demanding absolute green against a RED in-workspace base.
- `git add -A` in a sandboxed workspace (commits a giant scratch cache).
- One change spanning many subsystems broadly — closes targets, regresses the
  guard. Keep fixes surgical; gate the guard every time.
- Assuming all fleet tools work — smoke-test; let strong tools absorb the rest.
- Merging all stories at once without per-story re-gate.
- Chasing non-actionable failures.
- Fanning out shared-source work in parallel (collides at merge) — sequence it.
- Self-selecting the conductor seat, trialling yourself into it, or naming your
  own successor — the steward appoints and qualifies seats (§Seat authority).
- Handing scope to, or accepting scope from, a sibling conductor directly — route
  it through the steward and wait for the recorded boundary.
- Holding a steward seat and a conductor seat at once — self-review is not review.

## Command quick-reference
```sh
bashy sprint add/take/checkpoint/link/handoff/show <id>
bashy weave add --priority --points --tool --verify --body
bashy weave start --no-spawn --issue N        # allocate, then set up §5 isolation
bashy weave start --resume  --issue N -- <tool> "<body>"
bashy weave list / status N / log N -f / fleet / baton / cost --total
bashy weave wait --issue N --timeout 50m [--broker] &   # --broker: auto-route live gates
bashy weave say N "<msg>" / kill N --yes / salvage N / pull N / prune --stale --yes
```

## Foreman — the steerable session (Boss → Foreman → Worker)

A **conductor** fans a *whole backlog* out to parallel isolated workers, then
converges. A **foreman** is the complementary primitive for when the work is
**one live session that must be steered incrementally** — a single long-running
agent (or a small serial chain) you (the human **Boss**) or an outer conductor
adjusts mid-flight, rather than a fleet you batch-launch and gate. Pick foreman
when the task is exploratory, interactive, or a single cohesive build that
benefits from real-time correction; pick the fleet when the task decomposes into
many independent, gateable units.

`bashy foreman` is a **persistent, steerable session**. `bashy invoke` is the
degenerate one-turn case (it was called `chat` until 2026-07-12, which misled
agents into thinking it held a conversation — it does not: it invokes ONE agent,
ONCE, on one instruction). One session, two co-equal drive surfaces sharing one
on-disk state (`~/.bashy/foreman/<id>/` = `state.json` + append-only `commands`
queue + `ctl.sock` + `log` — server-less, inspectable, resumable, mirroring
weave's model):

- **Human drive (a TTY):** `bashy foreman [run <dag.md>]` → a readline REPL
  holding the session; typed lines are steering (`tell …`, `status`, `pause`,
  `resume`, `skip`, `stop`) or a plain message to the agent.
- **Agent drive (control channel):** a detached session + a control socket
  (generalized from weave's per-worker `ctl/*.sock`/`say`), driven from any
  process — an outer conductor, a cron, another foreman:

```sh
bashy foreman start [--detach] [run <dag.md>] --goal "<…>"   # begin a session
bashy foreman tell <id> "<msg>"          # steer the LIVE agent (same wire as weave say)
bashy foreman status <id> [--json] / list # reconciled state: idle|working|blocked|done
bashy foreman pause|resume|skip|prio|stop <id>
```

### `tell` steers a live agent — CHECK that it did

`tell` holds the agent open and types the message at it mid-turn. But an agent
whose tool declares no interactive launch cannot be held open, and there `tell`
falls back to **replaying** the conversation into a fresh one-shot. Both produce
an answer. They are not the same act: a replay cannot interrupt anything, because
by the time it lands the previous agent has exited.

`status --json` says which happened, and **you must read it** rather than assume:

```sh
bashy foreman status <id> --json | jq '{steering, steer_why_not}'
# {"steering": true,  "steer_why_not": null}          → the message reached a running agent
# {"steering": false, "steer_why_not": "…no interactive launch (steer_exec)"}
#                                                      → it was replayed. You did not interrupt anything.
```

If you needed to *correct an agent mid-flight* and `steering` is false, your
correction arrived after the fact. Re-issue it as the next instruction rather than
assuming it was absorbed.

Verbs and effect (the ycode Boss-control set, re-homed here): `pause` (finish the
current step → idle), `resume`, `stop` (cancel the current turn → exit cleanly),
`skip` (drop the current step, pick the next), `prio <target> p1|p2|p3` (re-rank a
backlog step), `tell <msg>` (freeform steer — interpret in context), `status`
(read-only). A `run <dag.md>` session drives a `bashy dag` task graph as its
backlog, pausing for steer between steps; `skip`/`prio` act on dag targets.

**When the Boss types a control intent in chat** ("pause for now", "skip this and
do X next"), append the equivalent verb to the session's `commands` queue for the
audit trail, then apply it — same as the CLI path, one reconciled state.

**Foreman vs. conductor:** a conductor may *drive a foreman* as one of its
workers (an outer loop steering an inner live session via `foreman tell`), and a
foreman may *launch a weave fleet* for a sub-batch. They compose; the choice per
layer is "batch-and-gate" (fleet) vs. "hold-and-steer" (foreman). Both obey the
same effect cap and the same "measured numbers are the only truth" principle —
`foreman status` is a lead until a gate reproduces it.
