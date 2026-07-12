# bashy coherence pass — the SDLC spine, the gate, and multi-agent coordination

## Context

Two agent sessions collided in one checkout today: one swept the other's staged submodule pins into
a commit (landing an untested `trap` regression that took the release gate 86/86 → 85/86), the other
found an unexplained edit in the working tree, and neither could see the other existed. **Communication
is not coordination** — two agents chatting politely still stomp one another's git index. What prevents
collision is **isolation → a claim → a merge gate**, in that order.

But the fix must not be another verb bolted onto a pile. bashy's agentic surface **grew piecemeal**, and
an audit says so plainly (below). So this is a **coherence pass first**: every extended feature examined
against one spine — **Plan → Code → Test → Deploy** — with gaps filled and redundancy removed, *then*
coordination lands inside that frame.

**This is a bashy feature, for any project.** Everything here must serve any project that adopts bashy —
nothing may assume a particular monorepo, a particular repo layout, or a forge. The project that surfaced
the problem is just the first user of the fix, not its scope.

---

## Ground truth — the audit (measured; do not re-derive)

| Stage | Verbs today | Verdict |
|---|---|---|
| **Plan** | `sprint`, `meet`, `kb`, `sdlc issue/brief` | ok |
| **Code** | `weave`, `supervise`, `foreman`, `chat`, `fanout`, `sdlc delegate` | **6 verbs — massively over-served** |
| **Test** | *(none)* | **the biggest hole** |
| **Deploy** | `sdlc promote/rollout/verify/resolve`, `dag deploy.md` | only reachable by adopting sdlc's whole forge control plane |

**The gate — the command that decides pass/fail — is spelled four incompatible ways:** a `::` string in
`supervise --task 'goal :: gate'` (`supervise/cli.go:39`), a `.agents/weave/suite-gate` file
(`weave_impl.go:1090`), a `Requires:` line in `dag.md`, and a `healthcheck:` key in `sdlc.yaml`.

**Two disjoint work-item lifecycles that never meet in code:**
- `sprint` — host-global `~/.bashy/sprint`, `backlog/doing/review/done` (`weave_story.go:391`), LLM-driven.
- `sdlc:*` labels — per-repo, `go/in-progress/qa/approved/done` (`sdlc/labels.go:19-26`), daemon+forge-driven.
- The "sdlc is the outer loop, sprint/weave the inner" story is **a YAML comment, not code**:
  `pkg/sdlc` does not import `pkg/weave`.

**Dead / undiscovered surface:**
- **`fanout`** — zero callers, zero skills, zero use-docs, **no atlas entry at all** (violating
  `command-atlas.md:37-38`'s own "no entry ⇒ build fails" rule), and its design doc concedes it collapses
  into weave. **Hard dead.**
- **`supervise`** — the only doc mentioning it says *"fallback only"* (`ci-failure-autorepair-plan.md:161`).
  No `list`/`status`/`resume`. Third report schema, third state dir.
- **`capability`** — dead as a verb, alive as a library (imported by `chat.go:17`, `meet.go:16`).
- **`meet`** — 1 skill mention. **Undiscovered, not redundant** (see Decisions).
- **`sdlc`** — 7,301 LOC with **exactly one consumer, and it is not bashy**; bashy never dogfoods it; the conductor skill
  mentions it **zero** times; its `act_runner` story has **zero code references**.

**Collisions:** `run` used 7 ways · `verify` 4 ways · `status`/`list`/`board`/`review` repeated across 5+
packages with **different JSON schemas** · 4 independent daemon loops · 4 append-only-JSONL memory
substrates · 3 state-root conventions (`~/.bashy/*`, `.bashy/generated/*`, `.agents/bashy/*`).

**The catalog already exists and already half-knows this.** `coreutils/pkg/atlas` +
`bashy commands --view group|tier|capabilities` has axes `class/subclass/group/tier/resolver/caps/effects`.
`group: orchestration` already lists 12 colliding verbs (`command-atlas.md:82`). Its one composite idiom —
`fleet-suite`: *"sprint (plan) → weave (isolate/run) → foreman (steer) → dag (targets)"* (`atlas.go:278`) —
**is a proto-SDLC axis, and it omits the verb literally named `sdlc`.** There is **no Plan/Code/Test/Deploy
axis anywhere.**

**The enforcement seat is reserved, and the universal channel is the shell — not docs.**
`agentos.go:989` composes an ExecHandler chain seeing the resolved argv of every command;
`dryrun.go:136-142` is a working refusal precedent (return without calling `next`);
`audit.go:100` reads `Decision: "allow", // allow-only until the policy engine ships`, and
`coreutils/pkg/policy/` contains only `audit/`. Meanwhile `install-agent` + `chat.forcedShellEnv` already
make bashy the shell for claude/codex/opencode/gemini/copilot/agy — so a shell-level rule reaches every
tool **without any of them reading a document** (which matters: ycode truncates instruction files at 4 KB
and reads `AGENTS.md` first, where bashy's is a 4-line stub; `coreutils/AGENTS.md` never mentions
`CLAUDE.md`; aider reads nothing).

**Live bugs found (fix in-flight):**
- **The advisor and nudge are OFF in a normal Claude session.** `nudge.go:157` gates on
  `weavecli.IsAgent()` → only `BASHY_AGENTIC` (set in one place, `run.go:146`), while
  `coreskills.DetectAgent()` *would* detect it. Two gates disagree; the weaker guards the channel.
- **`BASHY_AGENT_MANIFEST` is redacted from `context --json`** (`context.go:332-361` allowlist) — the
  manifest telling an agent what to do is hidden from the report meant to orient it.
- `bashy schedule` has no double-fire protection despite claiming idempotence (`schedule.go:345` vs `:368`).
- On Windows the one real flock is a **no-op** (`weave_lock_windows.go:11`).

---

## The project boundary is NOT `.git` *(user, this session — a first-class constraint)*

A modern project spans repos: libraries, sibling modules, a superproject, or simply several checkouts
anywhere on disk. **Our own collision is the proof.** The `trap` regression lived in **`sh`**; the gate that
would have caught it lives in **`bashy`**; the pin that carried it lives in the **umbrella**. A claim scoped
to one `.git` root would have prevented *nothing*.

**Today every subsystem assumes the opposite**, and it already hurts:
- `weaveRepoRoot(cwd)` = `git rev-parse --show-toplevel` (`weave_impl.go:320`); graph-contrib walks up to
  `.git`; `sdlc` state is per-repo. All single-root.
- The consequence is already recorded in the user's own kb (`vsc-pcts-harness-runbook`):
  *"Weave workspace clones do NOT hydrate submodules — build SUT binaries on a full checkout and ship them
  in."* **So weave's isolation — the primitive this whole plan stands on — is already broken for the
  multi-repo shape.** An isolated clone of `bashy` alone cannot build: it needs `../sh`, `../coreutils`,
  `../readline` (the `replace` directives + `.sibling-pins`).

**Design consequence — scope is a PATH SET, not a repo:**

- A **`Project`** is a set of roots, resolved by a `ProjectResolver` that (1) honors an explicit manifest if
  present, else (2) **infers** it from what the ecosystem already declares — git superproject/`.gitmodules`,
  `go.work` and `go.mod` `replace ../x`, `package.json` workspaces, Cargo/pnpm workspaces, bashy's own
  `.sibling-pins` — else (3) falls back to the git root, else (4) the cwd. Inference first; declaration only
  when inference is wrong. **A project may be any set of paths on disk.**
- **Conflict = intersecting path sets**, not equal repo roots. This generalizes cleanly: single-repo projects
  are the degenerate case, and it is the only model that catches "A holds `bashy`, B edits `sh`, and B's
  change breaks A's gate."
- Build on what exists: `context --json` already reports **`project_root`** and **`workspace_mount`**
  (`context.go`) — the notion is half-present and unused. Do not invent a parallel one.
- **`bashy gate` must be project-scoped too** — our own gate literally spans repos (`make test-bash` in
  `bashy` compiles `../sh`). A per-repo gate cannot express it.
- **`weave` isolation must hydrate the project, not the repo** — siblings/submodules included, or the
  isolated workspace cannot build and agents will "fix" that by reaching back into the live tree, which is
  the failure we are trying to end. This is a **prerequisite** for making weave mandatory for implementation
  work, not a follow-up.

## Decisions (user-locked)

1. **One vocabulary; join the stores.** Adopt **Plan/Code/Test/Deploy** as the spine. `sprint` stays the
   agent-facing plan/continuity layer; `sdlc` stays the forge/deploy adapter — but they are **joined in
   code** by a real key (sprint card ⇄ sdlc run), so the outer/inner-loop story stops being a comment.
2. **Deletions, with capabilities preserved:**
   - **`fanout` — DELETE.** Its capability (N agents, one shared context) is `meet`'s, and its own doc
     admits the collapse.
   - **`supervise` → `weave --in-place`.** Its sole honest differentiator is "live tree, not a clone" — a
     **flag, not a package**. The *delegate-and-supervise* capability is **preserved and improved**: weave
     already has the background run, the gate, `status`/`log`/`say`/`attach`, the gated `pull`, and the
     sprint continuity record. **This is the user's scenario** — "hand the chunking work to codex in the
     background while we keep talking, and supervise it to the goal because I hold the story" — and weave
     is the verb that does it. `supervise` has no `list`/`status`/`resume` at all.
   - **`capability` → library only** (drop from the front-door verb surface; keep the package).
   - **`meet` — KEEP as a front-door verb.** *(Reversing my own recommendation.)* `chat` is one agent, one
     turn; `weave` is parallel *isolated* work, not deliberation. Nothing else convenes a panel with turns,
     polls and convergence — **the user's second scenario**. Its problem is **discoverability (1 skill
     mention), not redundancy**. Fix by wiring it into the conductor skill and the Plan stage.
3. **`bashy gate` — the one Test verb.** One name for "the command that decides pass/fail", replacing the
   four incompatible spellings. Every stage transition consults the same gate definition.
4. **Coordination is POLICY, not a new orchestration verb.** It lands in `coreutils/pkg/policy/` — the
   package whose `Decision` field is already reserved for it — and is enforced in the ExecHandler.
   **Refuse on conflict only**: auto-claim silently; refuse a write **only when another agent holds a live
   claim**; `BASHY_CLAIM_FORCE=1` overrides and is recorded in the audit log.
5. **Isolation:** implementation work runs in a **weave workspace**; the live checkout is for the
   coordinator, QA/measurement and reads — and still requires a claim.
6. **Coordinator:** the **board is truth, the PM is a lease**. `bashy sprint` is already host-global with a
   heartbeat lease built for ephemeral LLM conductors (`weave_story.go:48-57`). The PM role rotates via
   `sprint take`/`handoff`. **Resumable, not special** — no single point of failure.

---

## Plan

### A. The spine — make the SDLC axis real *(no new code paths; the catalog leads)*
- Add an **`sdlc: plan|code|test|deploy|cross`** axis to `coreutils/pkg/atlas` beside `group`/`tier`, with
  `bashy commands --view sdlc`. The atlas is **coverage-test-ratcheted**, so this forces the question every
  redundant verb currently dodges: *which stage do you serve that nothing else does?*
- Fix the two atlas bugs the audit found: `fanout` has **no entry** (so the "missing entry fails the build"
  test does not actually cover front-door verbs — fix the test), and `supervise`/`capability` are missing
  from the doc's orchestration list (doc/code already drifted).
- Resolve the naming collisions in the atlas: `run` (×7), `verify` (×4), `promote` (×2), `status`/`list`/
  `board`/`review` (different JSON schemas for the same word).

### B. `bashy gate` — fill the Test hole *(the single biggest coherence win)*
- One gate definition per project, one command that runs it, one result schema (`bashy-gate-v1`).
- **Adapters, not rewrites:** `weave`'s `suite-gate` file, `dag`'s `Requires:`, and `sdlc.yaml`'s
  `healthcheck:` all resolve through it. `supervise`'s `::` spelling disappears with the package.
- This is what makes "sandbox-green ≠ mergeable" enforceable in one place instead of four.

### B2. `Project` — the boundary primitive *(prerequisite for C and for mandatory weave)*
- `ProjectResolver` in coreutils: infer the member roots from `.gitmodules` / `go.work` / `go.mod`
  `replace ../x` / workspace files / `.sibling-pins`; explicit manifest overrides; git root then cwd as
  fallbacks. Reuse `context --json`'s existing `project_root` / `workspace_mount` rather than inventing a
  second notion.
- **Fix weave's multi-root isolation**: hydrate the whole project (siblings/submodules) into the workspace.
  Until this lands, an isolated clone of a multi-repo project **cannot build**, and agents will reach back
  into the live tree — the exact failure we are ending. Blocking for "weave mandatory for implementation".

### C. Coordination — `coreutils/pkg/policy/coord` + enforcement
- **Scope is a path set** (the `Project` above). Conflict = **intersecting path sets**, not equal repo roots
  — the only model that catches "A holds `bashy`, B edits `sh`, B breaks A's gate".
- **Extract, don't invent.** Lift the blocking flock out of `weave_lock.go:25` (currently unexported and
  typed to `*weaveQueue`) into a reusable `WithLock`. Copy the heartbeat-lease shape from
  `weave_story.go:48-57` (TTL + heartbeat, *not* PID — an LLM session has no stable process) and the
  fencing epoch from `weave_baton.go:88`.
- **Fix the identity gap:** `principal.Resolver.Self()` (`principal/resolve.go:39`) already exists, but
  `BASHY_PRINCIPAL`/`EPISODE` are injected **only into children spawned by weave/chat** — a human-launched
  `claude` has **no id**, which is exactly why the two sessions were invisible. `coord` mints and persists
  one for a top-level session.
- `Claim{Scope{Host,RepoRoot,Kind}, Holder principal.Ref, Intent, Stage, AcquiredAt, Heartbeat, TTL, PID, Epoch}`
  in `~/.bashy/coord/`. Verbs: **`bashy claims`** (the missing host-wide "who is working, where, on what"),
  `bashy claim`, `bashy release`. Stale claims (TTL lapsed / PID dead) are reclaimable without `--force`.
- **`coordHandler` middleware** in `WireExec` (`agentos.go:989`), between `auditHandler` and
  `advisorHandler`, modelled on `dryrun.go:136-142` (return `interp.ExitStatus(N)` **without calling
  `next`**). Classify by `baseName(args[0])` — the existing idiom (`audit.go:96`). **Gate on
  `coreskills.DetectAgent()`, not `weavecli.IsAgent()`** — the same fix repairs the advisor and nudge,
  which are silently off today. Fill `audit.go:100`'s `Decision`: `allow|refuse|forced`.
  **The refusal is the documentation** — an agent that read nothing learns the rule on first violation.

### D. Consolidation
- Delete `pkg/fanout` + its registration (`agentos.go:351`).
- Add `weave --in-place`; port `supervise`'s gate semantics; delete `pkg/supervise` + `agentos.go:339`.
- Drop `capability` from the front-door verbs (keep the library).
- **Wire `meet` into the conductor skill** and the Plan stage — it is the panel primitive and nobody knows
  it exists.
- Pick **one** state-root convention (`~/.bashy/<verb>/`, which 7 of 9 already use) and file the strays.

### E. Join Plan → Code → Deploy
- A real join key: `sprint link` already ties a card to a weave issue (`weave_story.go`); add the missing
  half so an **sdlc run** points at its sprint card. Then `sdlc` genuinely becomes the forge/deploy edge of
  the same spine rather than a parallel universe.

### F. CI/CD auto-heal — make it actually run *(the cheapest dogfood proof)*
- **Human step (yours):** re-mint `CI_FAILURE_TOKEN` as a fine-grained PAT with **Issues: Read+Write +
  Metadata: Read** on the collector repo. Confirmed root cause:
  `Resource not accessible by personal access token (repository.labels)`. Every other part of the chain
  (workflow, collector repo, router, conductor shift roster, all six labels) is built and correct — **the
  loop has never run once.**
- Add a **token preflight** to `ci-failure-report.yml` that fails loudly with the fix, rather than dying
  inside `gh issue create`. A silent permission failure is how this stayed dark for its whole life.

### G. Codify — where it is actually read
- **`bashy/AGENTS.md`** (a 4-line stub today, read **first** by ycode and the **only** file Codex/OpenCode
  read) and the **top** of `bashy/CLAUDE.md` (above ycode's 4 KB truncation). Rewrite
  **`coreutils/AGENTS.md`** — today it guarantees a Codex agent there misses every rule.
- **`bashy/README.md`** — currently **zero** mentions of "agent". Add the working contract.
- **Runtime, which is what actually lands:** put the rule in `BASHY_AGENT_MANIFEST` (`agentos.go:155`),
  **un-redact it** (`context.go:332-361`), and add `claims`/`gate` to `recommended_commands` +
  `notes` (`context.go:203-217`) so the first call an agent makes states the rule.

---

## Verification

1. **Reproduce the collision, then prove it cannot recur.** Two shells, one repo: A claims + commits; B's
   `git commit` is **refused**, naming A, its intent and the remedy. `BASHY_CLAIM_FORCE=1` allows it and the
   override lands in the audit log.
2. **The invisible-session bug is closed:** a plain `claude`/`codex` session (no `BASHY_AGENTIC`) now gets
   an identity and appears in `bashy claims` from the *other* session.
2b. **The cross-repo case — the one that actually bit us.** A holds the project (bashy+sh+coreutils) and is
   running the gate; B, sitting in `sh`, attempts a write → **refused**, because the path sets intersect,
   even though the `.git` roots differ. Then: an isolated weave workspace for a multi-repo project
   **builds** (siblings/submodules hydrated) without reaching into the live tree.
3. **Stale claims reclaim** without `--force` after TTL.
4. **The advisor/nudge regression is fixed** — they fire under `CLAUDE_CODE_SHELL=bashy` (they do not today).
5. **`bashy gate`** produces the same verdict that weave's suite-gate, dag's `Requires:` and sdlc's
   `healthcheck:` produce today — adapters, not behavior change.
6. **`bashy commands --view sdlc`** places every verb in exactly one stage, and the coverage test fails on a
   verb with no stage — the ratchet that stops the next piecemeal addition.
7. **Both preserved capabilities, end to end:** (a) delegate the paused chunking work to `codex` in a weave
   workspace, in the background, while this session keeps talking — then supervise it to a gated merge;
   (b) convene a `meet` panel across ≥2 tools and converge.
8. **Auto-heal end to end:** a real failing run → collector issue → router claim → conductor repair in a
   weave workspace → gated merge.
9. **No regression:** `make test-bash` 86/86; `go test ./...` in bashy + coreutils.

## Non-goals

Agent-to-agent **messaging/inbox** — the weakest of the three mechanisms; it comes *after* claims prove out.
A durable workflow engine. Forcing a forge on anyone (`coord` and `gate` are **standalone-first**, like `kb`).
Rewriting `sdlc` (7,301 LOC with a real customer) — it gets joined, not replaced. Fixing `schedule`'s
double-fire and the Windows flock no-op — **file them**, do not scope-creep.
