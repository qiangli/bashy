# DHNT CI Failure Auto-Repair Plan

> **Superseded by** the umbrella `docs/ci-failure-autofix-pipeline.md`, which is
> the plan of record: it keeps this router/fixer/gate/handoff design but replaces
> the trigger (cloudbox webhook + outpost `/admin/repair` + `schedule` pull, not a
> self-hosted runner) and the selection (a **band ladder** L2→L3→human, not a fixed
> premium model). Read that first. This doc is kept for the role/handoff detail.

## Goal

Failed CI should become a managed repair queue, not a page a human has to
poll. The system should keep cheap deterministic work cheap, launch a
band-appropriate **fixer** only when there is real repair work, and run
code-changing agents in isolated workspaces so they do not clobber a live
checkout.

## Roles

### Router

The router is deterministic bashy automation. It may run every few minutes or be
called by a cloudbox/GitHub webhook. It does not diagnose, plan, or edit code.

Responsibilities:

- Poll or receive collector issues from `qiangli/dhnt-ci-failures`.
- Ignore issues labeled `repair-running`, `repair-paused`, `repair-failed`, or
  `repair-done`.
- Parse source repo, workflow, branch, SHA, failed run id, and failed run URL.
- Acquire a local lock/lease.
- Choose the on-shift fixer from the configured shift roster.
- Create a machine-readable handoff brief.
- Launch exactly one fixer session when claimable work exists.

### Fixer

The fixer is the **band-selected agent** that repairs one failing gate — attempt 1
at L2 (cheapest sufficient), attempt 2 at L3 (diagnosis); past the ladder it escalates
to a human (an L4 frontier model is not spent unattended). It owns the bounded fix,
evidence, and convergence for that one failure. A CI/CD fix does not need the full SDLC
`conductor` (decompose → delegate a fleet → converge); the conductor is the *escalation
target* for the rare fix that turns out to need orchestration. The fixer should not do
polling chores — the router does.

Responsibilities:

- Read the router handoff brief and collector issue.
- Check for an existing active or stale repair lease.
- Recover state if the previous fixer disappeared.
- Use `bashy weave` as the default isolated execution substrate.
- Assign workers only after defining measurable gates.
- Merge only through verified `weave pull` or an equivalent gated path.
- Update the collector issue with progress, blockers, and final outcome.
- Write a shift handoff note before going off shift.

### Workers

Workers are agentic coding tools selected by the fixer. They operate inside
weave workspaces, not the live source checkout.

Default pools:

- Even shift fixer `codex`: workers `claude, agy, opencode, aider`
- Odd shift fixer `claude`: workers `codex, agy, opencode, aider`

## Shift Roster

Fixers work in configurable shifts. Hour parity is only the default.

Configuration:

```sh
DHNT_CI_SHIFT_HOURS=1
DHNT_CI_SHIFT_ROSTER=codex,claude
```

Selection:

```text
shift_index = floor(unix_time / (DHNT_CI_SHIFT_HOURS * 3600))
fixer = DHNT_CI_SHIFT_ROSTER[shift_index % len(DHNT_CI_SHIFT_ROSTER)]
```

Examples:

- `DHNT_CI_SHIFT_HOURS=1`: codex and claude alternate hourly.
- `DHNT_CI_SHIFT_HOURS=2`: each fixer works a 2-hour block.
- `DHNT_CI_SHIFT_HOURS=8`: three human-style shifts are possible if the roster
  has three fixers.

## Handoff Protocol

A fixer must leave a handoff note before ending a shift or stopping work.
This is a gate, not a courtesy.

Minimum handoff fields:

- `fixer`: active fixer name.
- `shift_started_at`: timestamp.
- `shift_ends_at`: timestamp.
- `collector_issue`: issue URL/number.
- `source_repo`: repo under repair.
- `failed_run`: original failed CI run URL/id.
- `current_state`: one of `triage`, `assigned`, `working`, `verifying`,
  `blocked`, `done`.
- `weave_issues`: issue ids and states.
- `active_worker`: current worker/tool if any.
- `last_verified_gate`: command and result.
- `next_action`: the next concrete command or decision.
- `blockers`: concise list, empty if none.
- `updated_at`: timestamp.

The handoff note should be written to both:

- the collector issue comment thread, for durable shared state;
- local repair state, for fast recovery by the dragon scheduler/fixer.

Suggested local path:

```text
~/.bashy/ci-repair/<collector-repo>/<issue-number>/handoff.json
```

## Incoming Recovery Gate

Before starting new work, an incoming fixer must run a recovery gate.

Recovery gate:

1. Read the collector issue.
2. Read the latest local handoff if present.
3. Inspect repo weave state:
   - `bashy weave list --all`
   - `bashy weave status <issue>` for linked items
   - `bashy weave log <issue> --summary` where useful
4. Compare labels:
   - `repair-running`
   - `repair-failed`
   - `repair-paused`
   - `repair-done`
5. Decide one of:
   - continue from handoff;
   - recover from stale/missing handoff by summarizing current state;
   - pause as unsafe;
   - mark failed with blocker.

If the previous fixer was disrupted and no handoff exists, the incoming
fixer must first write a recovery summary before assigning or editing.

Recovery summary must answer:

- What issue is being repaired?
- What repo/workflow/run failed?
- What local weave issues/workspaces exist?
- Are any workers still running?
- Is there committed but unmerged work?
- What is the safest next action?

Only after this summary may the fixer launch workers or merge work.

## Execution Substrate

Default repair execution is `bashy weave`.

Why:

- each repair attempt gets an isolated workspace and branch;
- workers cannot clobber the live source checkout;
- terminal evidence, verify output, commits-ahead, and logs are persisted;
- merge happens through a gated `weave pull`;
- failed attempts can be reviewed, resumed, salvaged, or abandoned.

`bashy supervise` is fallback only.

Use fallback when weave cannot represent the required state, such as:

- a required gitignored artifact exists only in the live checkout;
- the fix must span multiple sibling repos in one atomic change;
- a repo depends on local sibling paths that are not isolated yet.

Even in fallback, prefer a temporary umbrella clone over the live checkout.

## Safety Gates

The auto-repair system must enforce:

- Never mutate the live repo directly from a worker.
- Do not start repair if the live repo is dirty unless explicitly allowed.
- Do not merge without a passing verify/review gate.
- Do not retry a failed auto-repair indefinitely.
- Do not allow two fixers to own the same collector issue.
- Do not allow an incoming fixer to continue stale work without writing a
  recovery summary.

## Desired Router To Fixer Prompt

The router should launch the selected fixer with a brief like:

```text
You are the on-shift DHNT CI repair fixer.

Use bashy and bashy weave. Do not edit the live checkout directly except through
verified weave pull. You are responsible for recovery, orchestration, evidence,
and handoff.

Before doing repair work:
1. Run the incoming recovery gate.
2. If no valid handoff exists, write a recovery summary to the collector issue.
3. Define the verify gate.
4. Then assign or resume workers in isolated weave workspaces.

Before ending:
1. Write a shift handoff note.
2. Include current state, linked weave issues, last gate result, blockers, and
   the next action.
```

## Immediate Next Implementation

Refactor the current dispatcher into:

- `ci-failure-router.sh`: cheap deterministic polling, claiming, shift
  selection, and fixer launch.
- `ci-failure-fixer-brief.md`: prompt template for the fixer.
- `ci-failure-handoff-schema.json`: optional schema for local handoff files.

The existing 5-minute `bashy schedule` job should call the router. A future
cloudbox webhook can call the same router immediately after a failed workflow.

## Current Wiring

The source repo workflow `.github/workflows/ci-failure-report.yml` is the
collector edge. On a failed `workflow_run`, it now:

- preflights `CI_FAILURE_TOKEN` against the collector repo;
- creates or updates the `ci-failure` issue in `qiangli/dhnt-ci-failures`;
- sends repository dispatch event `ci-failure-filed` to the collector repo with
  the collector issue number and failed run metadata.

The collector-side repair runner handles `repository_dispatch` event
`ci-failure-filed` in `.github/workflows/ci-failure-report.yml`. It must run on
a self-hosted runner labeled `dhnt-repair`, preflight the repair token, and run:

```sh
scripts/ci-failure-router.sh --once --issue "$COLLECTOR_ISSUE"
```

If the collector repo does not contain the scripts checkout, set repository
variable `DHNT_CI_REPAIR_ROUTER` to the absolute path of
`scripts/ci-failure-router.sh` on the `dhnt-repair` runner.

The router then claims the issue with `repair-running`, writes the local brief,
launches the on-shift fixer, and gives the fixer the gated merge helper:

```sh
scripts/ci-failure-gate.sh "$FAILED_RUN_ID" "$HEAD_BRANCH"
```

Required token scopes for `CI_FAILURE_TOKEN` on `qiangli/dhnt-ci-failures`:

- Metadata: read;
- Issues: read/write, including label access;
- Contents: write, so `repository_dispatch` can wake the repair runner.

The repair runner also needs a token usable by `scripts/ci-failure-gate.sh` in
the source repo with Actions: read, so the fixer can wait for the replacement
CI run instead of silently failing after the repair push.
