# Conductor runbook — fleet campaigns in depth

The deep companion to `SKILL.md`. The conductor skill is general (drive a fleet
to any verified goal), but the **test-driven campaign** is its canonical, most
demanding instantiation, so this runbook works it out end to end: given a known,
deterministic test suite with failing cases, drive a fleet of agentic coding CLIs
(claude, codex, opencode, aider, …) to **generate the code that makes every test
pass — without regressing any test already passing**, with one human or agent as
**conductor** and `bashy sprint` + `bashy weave` as the control + execution
planes. Substitute "target suite" with any exit-coded verifier and the runbook
holds unchanged.

The loop is pure red-green TDD, scaled out: **the failing tests are the backlog,
a passing run is "done", the suite is the gate, and the conductor never writes
the fix.** It applies to any campaign with the preconditions below — conformance
suites, a ported library's test vectors, a fuzz-corpus of reproducers, a
migration's golden tests, a coverage push, a spec's acceptance cases. Two
reference instantiations (the Bash 5.3 compatibility suite and the yash POSIX
scoreboard) are worked out in the Appendix.

The generic orchestration playbook is `bashy weave guide`; this is its
**test-driven**, conductor-led specialization.

---

## Preconditions (when a fleet TDD campaign pays off)

1. **A deterministic suite with per-case pass/fail.** You can name, run, and
   score individual tests reproducibly. Flaky or order-dependent suites must be
   stabilized first — a fleet amplifies flakiness into phantom regressions.
2. **A fast single-test repro.** Agents iterate dozens of times; if the only way
   to check one case is a 20-minute full-suite container run, fix that first
   (find or build a direct, no-container single-case path).
3. **A regression guard.** The currently-passing tests are the thing you must
   not break. Sometimes the same suite is both *target* (its failures) and
   *guard* (its passes); sometimes the guard is a second suite.
4. **Enough independent failures that decomposition wins.** For one or two
   failures, just fix them inline. The fleet earns its overhead when failures
   cluster into several disjoint root causes that can be worked in parallel.

If these hold, the rest of this runbook applies unchanged regardless of language,
repo, or what "passing" means.

---

## Terminology & hierarchy (official)

Four nested levels, largest to smallest. The first and third are the
human-facing names for the two anchors of the tooling (`task → sprint → run`);
the chain below makes the work-item level explicit.

> **Campaign → Sprint → Story → Run**

- **Campaign** — the **durable project goal** with measurable acceptance,
  spanning as many sprints as it takes, across time / hosts / agents. It is the
  *project*. Acceptance is one observable predicate ("the whole target suite
  passes **and** the regression guard is still green"). It carries a **continuity
  record** (resume brief) and a single **conductor lease** (one driver at a
  time). Done only when its acceptance holds on the **merged** result, not when
  the last sprint ends. Tooling anchor: the durable `bashy sprint` epic + baton.

- **Sprint** — **one bounded, gated pass** that takes a fixed backlog of stories
  from *measured-failing* to *verified-merged*. Four parts: a **backlog** (its
  stories), a **fleet** (tools assigned to them), an **authoritative gate**, and
  a **convergence step** (measure → merge → re-measure). A campaign is an
  **ordered series of sprints**, each **re-baselined** on the previous one's
  merged result, so the backlog shrinks every round. Tooling anchor: a **`bashy
  sprint`** card.

- **Story** — **one independently-deliverable, root-cause-coherent unit of work**
  with its *own* acceptance (the exact target cases) and its *own* verify gate.
  Stories in a sprint are **scope-disjoint** so they parallelize without
  collision. The agile "valued increment"; here, a fix-cluster. Tooling anchor: a
  **`bashy weave` issue**.

- **Run** — **one execution of a story by one tool in one isolated workspace**:
  the atomic unit of agent work, where the analysis and code happen. A story may
  take **several runs** — reassignment, a re-drive after an early stop, or
  salvage of a watchdog-kill — but it is *done* only when one run passes its gate
  and merges. Tooling anchor: a **`bashy weave start`**.

**Invariants at every level:** each has an *acceptance* (a measured predicate)
and a *gate* (the command that decides it); nothing is "done" on assertion, only
on a reproduced measurement; and convergence always ends with a **re-measure on
the merged tree**, never on a workspace in isolation. The conductor drives the
campaign; the fleet executes the runs.

---

## 0. The shape of the work (why a fleet, why a conductor)

A wall of failures is rarely a wall of independent bugs — it is usually a small
number of root causes, each surfacing in many cases. That is the profile a fleet
eats well: decompose into disjoint clusters, fan one agent per cluster across
isolated workspaces, verify, merge, re-measure, repeat.

The conductor never writes the fix. The conductor: reads the harness, measures,
groups, files stories, builds the **isolation + gates**, launches and
**monitors** the fleet, **salvages** killed runs, **gates every merge**, and
**iterates**.

Operating principle throughout: **the queue + the measured numbers are the only
truth.** A worker's prose or commit message is a lead until the exact repro
reproduces it. Echo measured numbers verbatim; never trust "submitted".

---

## 1. Read the harness first

Before measuring anything, understand exactly how a single case is scored —
because the agents' verify gate is built from it. Establish three things:

- **The scoring contract.** How does one test report pass vs fail? (an exit code,
  a `.trs`/TAP line, a golden-file diff, a JUnit XML node). The gate will
  grep/parse exactly this.
- **The fast single-case repro.** The exact command that runs *one* named case
  against the *built artifact*, runnable from a workspace with no heavyweight
  infra. This is what every agent loops on. If it doesn't exist, building it is
  the conductor's first job — it is the single biggest multiplier on agent
  throughput.
- **Which artifact is measured.** Build and gate the *same* binary/package/bundle
  the canonical scoreboard measures, with the *same* transforms (env vars,
  filters, skips). A gate that measures a different artifact, or skips a filter
  the harness applies, false-passes or false-fails silently.

Identify the **two roles** a suite can play: *target* (cases to turn green) and
*regression guard* (cases that must stay green). One suite may serve both; a
campaign may use two suites (one as target, one as guard).

---

## 2. Measure the baseline

Run the suite(s) and record two numbers: the **actionable failing set** (the
work-list — cases + a one-line description each) and the **passing count** (the
regression anchor).

**Filter the failing set down to what is actually yours to fix.** Real suites
carry failures that are not bugs in your code: environment-specific cases,
known-flaky cases, upstream-default differences, platform ceilings. Subtract
those up front (e.g. by differencing against a reference oracle: "cases the
reference implementation passes but ours fails"). Chasing non-actionable failures
wastes whole runs and can introduce regressions when an agent "fixes" something
that was never broken.

**Note environment-divergent cases** — ones that pass in one environment (the
host) but fail in another (the canonical container/CI). Probe each target on the
agent's environment before assigning it, so a run's local gate doesn't
false-pass; the conductor re-verifies those in the canonical environment at the
end.

Snapshot the baseline. Every later sprint re-runs this against the improved tree
to get the shrinking remainder.

---

## 3. Analyze + group by root cause (not by raw count)

Cluster the failures by the **code path that fixes them**, so each agent owns a
coherent area and agents don't collide. Rules that pay off:

- **A big cluster with one root cause stays one story** — don't shard a single
  fix across agents (they'll duplicate and conflict). One fix verified across
  many cases is one story.
- **A grab-bag of unrelated single cases groups by sub-mechanism**, not
  alphabetically or by file.
- **Note where two clusters share a source file.** They can still run in
  parallel, but they must **merge sequentially** with a re-gate (§9), and you
  should tell each story to keep its edits localized so the fix-sets combine.
- **Size to ~30 min of agent work per story** (the points-8 cap). If a cluster is
  bigger, split it; if a "fix" needs cross-cutting investigation first, do a
  one-pass investigation and file one story per discovered root cause.

---

## 4. Create the sprint + the stories

Wrap the campaign in a **`bashy sprint`** card (the durable plan/continuity
layer) and file one **`bashy weave` issue** per story.

```sh
bashy sprint add "<campaign goal>" \
  --acceptance "<the one measured predicate: target suite green AND guard green>" \
  --column doing --epic <campaign-name>
bashy sprint take 9 --as conductor                 # claim the single-driver lease
bashy sprint checkpoint 9 --continuity "<baseline + plan + blockers>"

bashy weave add "<story title>" --priority p0 --points 8 --tool codex \
  --verify "$(cat verify-gate.sh)" --body "$(cat story.md)"   # × N
bashy sprint link 9 --repo <repo> --task <issue-id>           # link each run to the sprint
```

Also claim the weave **baton** (`bashy weave baton take --as <you>`) — the
queue-scoped single-driver lock. Re-write it (`sprint checkpoint` / `weave baton
write`) after every meaningful action; **monitoring you don't record is lost the
moment you drop**, and the baton is what lets any handoff (planned or a crash)
resume cleanly.

### The story-body contract (what makes agents succeed)

Each body, in order: **SETUP** (workspace + any private-dependency rules, §5) ·
the **single-case repro** recipe (§1) · **GOAL** with the exact target case
identifiers and a one-line description of each · a **root-cause hypothesis**
pointing at the likely package/file · **SCOPE** (a directory allowlist, disjoint
from sibling stories) · the **GATE** (§6) · **commit discipline** (named files;
never `git add -A`) · a **blockers escape** (commit the partial + a
`<TOPIC>-BLOCKERS.md` after three honest attempts).

**Specificity drives yield.** Pasting the exact failing cases + the local repro
closes several times more per run than "make X match the reference". Embed any
hard-won findings (e.g. a bisect result naming the file that caused a prior
regression) so the next agent skips the trap.

---

## 5. The critical infrastructure (where naive setups silently break)

Weave gives each run an isolated *git clone*. That isolation has four gaps that
each waste a run if not closed **before** launch. They generalize beyond any one
project.

1. **Shared mutable build dependency — the #1 trap.** If the fix lives in a
   dependency the build resolves through a path that weave **shares across all
   workspaces** (a `go.mod` `replace => ../dep`, a vendored submodule, a
   workspace symlink), then every agent edits *one* directory and they clobber
   each other — and each `weave start` may re-sync it, wiping edits. **Fix:** give
   each workspace a **private copy** of that dependency and repoint the build at
   it *without polluting the merge*:
   ```sh
   cp -R <shared-dep> $W/.dep-private
   # repoint the build (edit the replace/path) ...
   git -C $W update-index --skip-worktree <the-manifest>   # branch keeps the shared path; build uses private
   printf '\n.dep-private/\n' >> $W/.git/info/exclude
   ```
   Repoint via the manifest's existing redirect mechanism, not a global override
   that drags in unrelated dependencies. The real fix then lives **committed
   inside each private copy** for the conductor to apply at merge. *(In the bashy
   tree the dependency is the `sh` interpreter engine reached via `replace
   mvdan.cc/sh/v3 => ../sh`; the private copy is `.sh-private`.)*

2. **Test data that isn't in the git clone.** Fixtures, corpora, golden files, or
   generated assets that are **gitignored** (common for clean-room or large test
   data) are absent from a clone. Copy them into each workspace — a **copy, not a
   symlink**, because in-tree-writing test runners would race a shared directory
   across parallel agents.

3. **A RED baseline in the workspace.** The in-workspace test environment can
   differ from the canonical one and produce failures unrelated to the work
   (missing build artifacts the canonical tree has, a different locale, absent
   helpers). A clean workspace that scores *below* the canonical baseline will
   false-fail every agent. Two fixes, both valid: (a) gate on **"no NEW failure
   beyond the known baseline set"** rather than absolute green (the "gate around a
   RED base" pattern); (b) better, **make the workspace environment canonical**
   (generate/stub the missing pieces, guarded so it's a no-op in the real tree)
   so the in-workspace baseline is a true green. An agent will often produce fix
   (b) itself — adopt it into the repo and the whole round gates clean.

4. **Tool scratch pollution in commits.** Sandboxed tools write caches/scratch
   into the workspace (e.g. a sandbox's module cache, editor litter, `.aider*`
   files). A salvage `git add -A` sweeps thousands of junk files into a commit.
   **Always commit named source paths**, never `-A`, and add the scratch dirs to
   the workspace exclude.

---

## 6. The verify gate (the merge's backbone)

Bake a `--verify` command into every story. Weave runs it (`bash -c`) in the
workspace at terminal time; non-zero blocks `weave pull`. Three clauses:

```sh
<build the artifact>            || exit 2          # 1. it still builds
<run each target case>          ; assert all PASS  # 2. the story's goal is met
<run the regression guard>      ; assert no NEW failure beyond the known baseline   # 3. nothing broke
```

The third clause is **non-negotiable** and the most common thing teams get wrong:
a gate of only "the new tests pass" (or only `go test`) misses regressions in the
broader guard suite, and **broad changes routinely close target cases while
nicking a previously-passing case.** Bake the regression guard into `--verify` so
most regressions die before they reach a merge — and the conductor *still* re-runs
the guard in the **real** repo post-merge (workspaces can't always be trusted).
Gate on "no failure beyond the known baseline set" so a RED-base environment
(§5.3) doesn't false-fail good work.

---

## 7. Assign + launch the fleet

Match tools to stories by the **report card** (below), biggest/hardest to the
strongest tool. Pre-seed each tool's trust/permission cache during prep, set
watchdogs, background each launch:

```sh
bashy weave start --resume --issue N --max-runtime 45m --mem-limit 12g \
  -- <tool> <recipe> "<body>" &
```

Recipes (typical): `claude --dangerously-skip-permissions "<body>"` · `codex exec
--skip-git-repo-check --sandbox workspace-write "<body>"` · `opencode run
"<body>"` · `aider --yes-always --no-check-update --message "<body>"`. Calibrate
`--mem-limit` to the build's link peak (a large native link can hit several GB).
Note that watchdog timers count **awake** wall-clock — a sleeping laptop skews the
displayed duration (hours against a 40 m cap is not a bug).

### Tool report card (update as evidence accrues)

Tool reliability is **campaign-independent enough to carry forward**, and it is
lopsided — assume some of your fleet is dead weight and let the strong tools
absorb the rest via reassignment (§8). Patterns observed across campaigns:

- **codex** — the workhorse. Localized, honest fixes (declines to commit when its
  change regresses the metric); fastest; often the first to a clean,
  verify-passed result. Best default pick, especially on a clean base.
- **claude** — strongest on deep, multi-file / parser-level work. Frequently hits
  the runtime cap mid-fix — **salvage its uncommitted work** (§8).
- **opencode** — usable only on a **tight, well-scoped** story on a clean base;
  broad attempts have regressed the guard, and it can exit 0 having done nothing.
  Keep it on small stories; watch the artifact, not the exit code.
- **agy** / **aider** — both **washed out** in these campaigns (one
  non-functional with zero output; one stuck in a reasoning loop with no
  commits). **Smoke-test every tool on a trivial prompt before trusting it**, and
  kill reasoning loops rather than waiting.

---

## 8. Monitor — event-driven, and actively

Use weave's own waiting, **not** host `sleep` loops (they time out the conductor
and add no signal). One backgrounded wait per story wakes the conductor exactly
when a run terminates:

```sh
bashy weave wait --issue N --timeout 50m &     # one per story; fires on terminal state
```

On every wake, **act**:
- **submitted / killed** → measure against the GOAL; never trust the state.
  Re-run the exact target repro. "submitted" ≠ done — agents stop early, and
  watchdog-killed runs often hold real **uncommitted** work.
- **salvage watchdog kills** — a run killed at `--max-runtime` usually left a
  large diff uncommitted that closes most of the cluster. Build it, measure,
  commit the **named source files** (not scratch), gate it.
- **reassign / work-steal** — a finished or dead agent gets the next story; a
  no-op story gets re-driven with a sharper prompt or a **different tool** (`weave
  start --issue N -- <other-tool>` changes the owner). Dead tools' stories get
  absorbed by the strong tools this way.
- **blocked on a prompt** → answer it (`weave say N "1"`); first verify the block
  is live (a child process in a TTY wait), not stale scrollback.

**Never measure the suite on the host while agents compile in parallel** —
per-test timeouts flake under load and read as phantom regressions. Measure each
run in isolation after it terminates.

---

## 9. Converge — sequential, gated merge on a review branch

Merge verified stories one at a time onto review branches (no push, no
dependency-pin / submodule bump without explicit human OK):

```sh
git -C <repo>           checkout -b <campaign>-merge
git -C <shared-dep-repo> checkout -b <campaign>-merge      # if the fix lives in a dependency
```

For each story, in order:
1. Generate a **source-only** patch from the run's private workspace (`git diff
   <base>..HEAD -- <source-dirs>`) — never scratch/cache or test artifacts.
2. `git apply --3way` it. Stories that share a file **3-way merge cleanly** when
   applied sequentially; resolve any conflict by **combining** fix-sets.
3. Apply any non-dependency-side commits by **named path**; leave unrelated
   working-tree edits untouched.
4. **Re-gate**: rebuild against the merged dependency, run the **regression guard
   in the real repo**, and re-measure every merged target case.

**Watch for cross-cluster ripple.** Combining stories that each pass in isolation
can still drop a case to an interaction between their edits. The combined
re-measure (step 4) catches it; fold the recovery into the next sprint rather than
blocking the merge. Bisect any guard regression against the pre-merge commit
before accepting the round.

---

## 10. Iterate

Re-run the suite **against the merged review branch** to get the shrinking
remainder, then re-divide and re-sprint on the now-cleaner base. Each round is
structurally cleaner than the last: workspaces clone the **merged** base (so the
gate baseline is true-green via any §5.3 fixes already adopted), and every round's
bisect findings are embedded into the next round's story bodies (agents skip prior
traps). Repeat until the actionable failing set is empty; verify the
environment-divergent cases and any cross-cluster ripple in the canonical
environment in a final consolidation pass.

At round end, write the durable residue into the **host kb** (`bashy kb`) —
the collective memory of all agents on this host across all repos. Campaign
notes die with the campaign; the kb is where the cross-campaign lessons live
(the tool report card patterns above are exactly this genre). `bashy kb retro
<terms>` structures the decision: add / update / supersede / validate / noop —
distilled strategy with evidence, failures phrased as guardrails, never
transcripts. Workers already saw the relevant pages (weave drops KB.md into
each workspace), so a validated kb page is knowledge the NEXT campaign's
fleet gets for free.

---

## Anti-patterns (each costs a round)

- Hand-rolling `sleep`-loop monitoring instead of `weave wait`.
- A gate that demands absolute green against a RED in-workspace base.
- `git add -A` in a sandboxed workspace — commits a giant scratch cache.
- One change spanning many subsystems broadly — closes target cases but regresses
  the guard. Keep fixes surgical; gate the guard every time.
- Assuming all fleet tools work — smoke-test; expect washouts; let strong tools
  absorb the rest.
- Merging all stories at once without per-story re-gate — a clean textual merge
  can silently regress a case; sequential gating + combined re-measure catches it.
- Chasing non-actionable failures (environment/flaky/not-our-bug) — wastes runs
  and invites regressions.

---

## Command quick-reference

```sh
# measure (project-specific suite commands)
<run target suite>  ;  <run regression-guard suite>
# sprint (plan / continuity layer)
bashy sprint add/take/checkpoint/link/comment/show <id>
# weave (per-repo execution)
bashy weave add --priority --points --tool --verify --body
bashy weave start --no-spawn --issue N      # allocate workspace (then set up §5 isolation)
bashy weave start --resume  --issue N -- <tool> "<body>"   # launch a run
bashy weave list / status N / log N -f / fleet / baton
bashy weave wait --issue N --timeout 50m &  # event-driven monitor
bashy weave kill N --yes / salvage N / pull N / prune --stale --yes
```

---

## Appendix: two reference campaigns

Both instantiate this runbook unchanged; only the suite-specific cells differ.

| Runbook concept | Campaign A — Bash 5.3 compatibility | Campaign B — yash POSIX scoreboard |
|---|---|---|
| **Target suite** | `make test-bash` (Bash 5.3's own `tests/` fixtures, ~86) | `make test-yash` (`scripts/yash-scoreboard.sh`, yash `*-p.tst` corpus) |
| **Regression guard** | the *same* suite (its passing fixtures) | `make test-bash` (must stay 86/86) |
| **Actionable-failure filter** | failing fixtures vs. the 86 expected | "bashy-specific": cases real `bash` passes but `bin/bash` fails (both-fail = posix-vs-bash noise, ignored) |
| **Scoring contract** | `N passed / M failed` summary + per-fixture diff | per-case `.trs`: `%%% OK[PASSED]: <suite>-p.tst:<LINE>:` / `%%% ERROR` |
| **Fast single-case repro** | `make test-bash TESTS="<name>"` | `LANG=C sh run-test.sh "$PWD/bin/bash" <suite>-p.tst` then grep the `.trs` line (no container) |
| **Artifact measured** | `bin/bash` (the lean drop-in, `./cmd/bash`) | same — `bin/bash`, not `./cmd/bashy` |
| **Shared-dependency isolation (§5.1)** | private `.sh-private` per workspace; `replace mvdan.cc/sh/v3 => …/.sh-private` + `skip-worktree go.mod` | same |
| **Gitignored test data (§5.2)** | `external/bash-5.3/tests` (symlink → source tree) | `.yash-tests/` (runtime clone) + `external/bash-5.3/tests` |
| **RED-base fix (§5.3)** | n/a (canonical tree) | stub the missing build artifacts (`y.tab.c`, `config.h`, `version.h`, `examples/loadables/Makefile`) in `test-bash-helpers`, guarded |
| **A representative result** | drove the suite to all-green and held it as the guard for Campaign B | 97 bashy-specific failures → 31 in one merged sprint (91% → 94%), guard held 86/86 |

The headline difference between the two: in Campaign A the one suite is *both*
target and guard; in Campaign B the yash scoreboard is the target and Campaign A's
now-green suite is the **regression guard** — which is exactly why a campaign's
output (a passing suite) becomes the next campaign's safety net.
