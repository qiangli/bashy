# Sprint 562 master execution plan: fence corpora coverage and repair

Review date: 2026-11-03. Planning only: no sprint takeover, test dispatch,
toolchain provisioning, product change or release action was performed.
Develops Sprint 562 under `plan-clean-baseline-campaign.md`.

Revised the same day at the operator's direction: the by-example corpora
planned in the `fence-corpora` epic (Sprints 391–396) are consolidated into
this sprint as its first phase, and must be green. The four existing stories
(B1–B4) become the last phase. Operator follow-up the same day: stories E1–E4
are added to the card, Sprints 391–396 are closed as superseded (no work was
delivered under them), and every example targets every platform.

## Main goal

**Bashy is all you need, on every platform.** A user writing Python, Go,
TypeScript, C#, C/C++, Rust or PowerShell inside Bash# fences gets the same
working result on Linux, macOS and Windows from the one `bashy` download.

One frozen Bashy candidate on which **every official example the fence
corpora promised is covered** — inventoried from a pinned source, generated
as a fenced Bash# case, and **passing** against its native oracle — and on
which every promised fence row then passes its declared platform/provider
matrix, with reproducible prerequisites, reconciled denominators and no
unexplained skip, xfail or unavailable cell. Defects are fixed at their
owning layer. Optional providers (sprint C) stay optional.

The corpora are a **permanent regression guard**, not a one-off check: once
green they stay a fatal gate for every later fence, engine or toolchain
change (see "Regression guard" below).

One shared runner, existing harness conventions, no fence-framework rewrite,
no release.

Card state at review: backlog, unclaimed, 0/4 stories closed, PRIORITY-FIRST.
Sprints 391–396 are backlog with no stories and no implementation; none of
their corpus directories exist in `bashsharp-tests`. Sprint 561 (A) has
campaign priority and is active; claim shared test hosts before use.

## Consolidated scope from the fence-corpora epic

The archived epic specs (`docs/sprint-391…396-*-plan.md` in the archived
umbrella) define the sources. They are reference material: this plan keeps
their sources and oracles and drops their per-sprint process.

| Source (pinned upstream) | Corpus dir in `bashsharp-tests` | Oracle | Was |
| --- | --- | --- | --- |
| Python Tutorial (`Doc/tutorial/*.rst`, CPython tag) + `00-boundary` | `python-by-example/` | native CPython | 391 |
| Go by Example (85) + Go Tour (97), generated fenced | `examples-fenced/`, `tour-fenced/` | existing oracle hashes / `tests/tour/results.tsv` | 391 |
| TypeScript handbook + playground (TypeScript-Website commit) | `typescript-by-example/` | reviewed golden; compiler error codes | 392 |
| C# tour, tutorials, fundamentals (dotnet/docs commit) | `csharp-by-example/` | native .NET | 393 |
| C and C++ references (MicrosoftDocs/cpp-docs commit) | `c-by-example/`, `cpp-by-example/` | native clang C17 / C++20 | 394 |
| Rust by Example + The Rust Book listings | `rust-by-example/` | rustc from the same provisioned toolchain | 395 |
| PowerShell 101, deep dives, `about_` (PowerShell-Docs `reference/7.6`) | `powershell-by-example/` | native pinned pwsh | 396 |

Kept from the epic: generated (never hand-written) inventories in which every
upstream example has a disposition; per-case provenance and licence
attribution; the tour's `cases.tsv` + `.expected` (`# rc=N`) conventions;
byte-exact file capture; separate stdout/stderr where streams matter;
deterministic seeds; one fatal aggregate section in `harness/run.sh`; the
existing three-mode Go-by-example gate unchanged.

## What "green" means

- **Case:** an example that runs as a fenced case within the fence's declared
  contract. Every case must **PASS** in each required mode. No xfail, no skip.
- **Non-case disposition** (not counted as a pass, always listed): prose or a
  fragment, interactive, browser/DOM, MSVC-only, Windows-only on a non-Windows
  leg, network/environment-dependent, intentionally non-compiling (these
  become **diagnostic cases** that must match the official error).
- **Missing Bash# feature:** an example that needs a fence feature Bash#
  does not support yet (for example TypeScript top-level imports, classes as
  handles, JS/TSX fences; see the epic's linked cards). Finding these is a
  purpose of the corpora: each is a feature Bash# must support, not a reason to
  narrow the contract. It is never hidden or counted as a pass; it is recorded
  with its owning story and implemented (broad ones costed and reviewed
  first). The sprint does not close with an unimplemented in-scope feature.

## Regression guard

The point of the corpora is to keep future changes from silently breaking a
working example. E1 builds the guard in from the start:

- **Fatal and mandatory:** the aggregate section in `harness/run.sh` cannot be
  skipped; a missing runner, manifest, mode record or toolchain fails it.
- **Denominator ratchet:** each corpus commits its expected case count and
  PASS count per mode and platform. A lower count, a case turning into a
  non-case disposition, or a new contract gap fails the gate; raising the
  count is a reviewed change. Removing a case requires a reviewed reason in
  the inventory.
- **Pinned sources:** upstream commits and toolchain hashes are pinned;
  re-pinning is an explicit reviewed operation that regenerates byte-identical
  cases first, then reviews any diff. `--pin` never accepts output from the
  implementation under test automatically.
- **When it runs:** on every change to `../sh` polyglot/interp/lower,
  `../bashsharp`, bashy fence rows or `yoke/external` provisioners, on the
  self-hosted test hosts (not the dev box, not hosted CI by default); and in
  the release checklist. The fast inner loop runs one corpus or one case.
- **Bounded:** per-case and per-corpus wall-time limits, recorded; a slow
  case is diagnosed, never given a larger timeout to pass.

## Story chain

Phase 1 — example corpora (proposed new stories, grouped to stay minimal):

| Order | Story (proposed) | Gate to start | Exit evidence |
| --- | --- | --- | --- |
| E1 | Shared fence-corpus runner + inventories for all seven sources | claim card | runner in `harness/run.sh` (fatal) with the count ratchet; every upstream example has a disposition; pins, licences |
| E2 | Python + fenced Go corpora green | E1 runner + Python/Go inventory | all cases PASS; Go 85 + 97 accounted for |
| E3 | TypeScript + Rust corpora green | E1 | all cases PASS; diagnostic cases match official codes |
| E4 | C/C++ + C# + PowerShell corpora green | E1 | all cases PASS, Windows leg for C#/PowerShell |

E2–E4 may run in parallel once E1's runner exists; each owns its fixes.

Phase 2 — last phase, existing stories, now including the new corpora:

| Order | Story | Gate to start | Exit evidence |
| --- | --- | --- | --- |
| B1 `feae8aa96a9a` | inventory + MVP matrix | E1–E4 done | fence × platform × provider matrix, commands, gaps |
| B2 `ca8cc67e0287` | reproducible baselines | B1 | measured result or blocker per required cell |
| B3 `6a4f4591bc8e` | repair remaining defects | per-finding baseline from B2 | red/green, original cases rerun |
| B4 `af5a35713e47` | verify frozen candidate | B3 + consumed A3/C3 fixes | full matrix rerun from clean env, receipts |

## 0. Establish ownership

- Designated manager runs `bashy sprint take 562`; add E1–E4 to the card
  (operator approval); link this plan; checkpoint each step.
- Leave Sprints 391–396 unchanged (campaign rule: historical cards are not
  falsely closed); add a one-line "consolidated into 562" note to each.
- Claim the self-hosted Linux, macOS and Windows test hosts before use.
  Corpora provision foreign toolchains: never run them on the dev box.

## Phase 1 — example corpora

### E1 — runner and inventories

- One runner, `bashsharp-tests/tools/fence-corpus/check.sh`, plus thin
  per-language preparation scripts. Reuse the tour's case format and the
  existing Go-by-example supervision/timeouts; no new process supervisor.
- Modes: **interpreted** (`bashy --bashsharp`) is required for every case;
  **compiled** (transpile → pinned Go SDK build → run) is required wherever
  the lowering already supports that fence row. E1 records which rows those
  are; it does not extend lowering.
- Inventory generator per source at a pinned upstream commit: every code
  block or file listed, each with a disposition; `pin.tsv` records source
  commit, licence and per-case provenance. Unaccounted examples fail.
- Pins: the toolchains bashy already provisions (`islandToolchains`,
  `yoke/external`). Raising a default interpreter version is out of scope; an
  example that needs a newer language version is a recorded disposition.
- Wire one fatal aggregate target into `harness/run.sh`.

### E2–E4 — corpora green

For each corpus: generate cases, run the native oracle, run both required
modes, then classify every non-pass as fence defect / contract gap / A cause
(→561/A3) / C cause (→563/C3) / prerequisite. Fix fence defects at the owner
(`../sh` polyglot/interp/lower, `../bashsharp`, `internal/agentos`,
`../yoke/external`), with a focused regression and the original case rerun.
Implement missing Bash# features as defined above. Exit when every case
PASSes on the platforms in the current rollout step. Commits carry `Sprint: 562` and the owning story trailers.

## Phase 2 — last phase (B1–B4)

### B1 — inventory and matrix

One table from code, not prose: the registry (`sh/polyglot/languages.go`,
`text_rows.go`, bashy `RegisterLanguage` rows), the toolchain pins, and the
corpora — the new by-example corpora, `bashsharp-tour/cases.tsv`,
`harness/run.sh`, `make smoke-dag-*`, focused fence Go tests.

Rows found at review: in-process `bash sh skill dag tmpl`; code `python
typescript javascript rust c cpp go powershell csharp`; manifest `cargo
pyproject gomod cmake makefile package`; text `dockerfile tf k8s helm`; fleet
`model tool agent`; reserved stub `compose` (not promised).

Classify each cell **B-required** (the language contract, explicit refusals,
fleet rows with the tour's local recorded transport) or **C-optional** (live
podman/tofu/k8s/helm targets, network, real model APIs, Bun, host MSVC; B
requires only the explicit absent-provider refusal). Record declared OS/arch,
pins, hashes, the measuring command and the denominator. **Every existing
tour `xfail`/`ci:os=card` marker is a gap to account for.**

### B2 — baselines

Provision through supported mechanisms; run each corpus once per platform on
the baseline with bounded wall time and isolated `HOME`/`TMPDIR`/catalog.
Classify every non-pass; missing prerequisites are blockers, never skips.
Consult 539 F2/F3 and 560 3063 causes only when a current failure matches.

### B3 — repairs

Remaining defects outside the by-example corpora (text/manifest/fleet rows,
tour xfails, platform gaps): smallest causal fix at the owner; keep
malformed-engine rejection, no-fallback and effect boundaries; no weakened
assertion, blanket exclusion, new xfail or timeout growth.

### B4 — verify the frozen candidate

Freeze the candidate (bashy commit, every pin, product-wrapper binary hash,
toolchain and provider digests). From a clean cache on each declared platform
run the full matrix — all by-example corpora plus the rest — once, then again
with C providers absent. Reconcile every denominator. Any failure returns to
its repair story. Per-platform receipts in one shared evidence record; the
manager independently accepts. No release.

## Cost and risk (discuss before Phase 1 starts)

- **Size.** The sources hold thousands of examples (the C# docs tree alone
  held 665 snippet files in October; Rust by Example, the Book, the C/C++ and
  PowerShell references add hundreds each). Generation is mechanical; the
  cost is in classification and fixes.
- **Missing Bash# features** are the main cost: the epic already names
  TypeScript imports, class handles, JS/TSX fences, streaming, tsconfig
  analysis, and similar Rust/C#/PowerShell limits. Each is feature work; each
  broad one is costed and reviewed with the operator before implementation.
- **Compiled mode** for rows without lowering would be new lowering work and
  is excluded by the E1 rule above.

## Platform rollout

Target: every example on every supported OS/arch. Rollout, as time permits:

1. **Step 1 (this sprint's first gate):** Linux for every corpus; Windows
   additionally for C# and PowerShell.
2. **Step 2:** Windows for the remaining corpora.
3. **Step 3:** macOS for every corpus; then each remaining arch.

Each step adds its cells to the ratchet; a platform once green stays a gate.
A platform-specific failure is a defect to fix, not a permanent exclusion.
Examples that are inherently OS-specific upstream (Windows-only API samples)
are dispositions on the other OSes only.

## Recommendations for open decisions

1. TypeScript: one compiler leg, the pinned default; a TypeScript 7 leg is
   advisory evidence only.
2. Tour "by-example" chapter and promoted tour rows: optional, not a gate.
3. Bun, MSVC, `compose`, Java: optional or out of scope.

## Stopping rules

- Deadline hit = retained incomplete evidence + diagnosis, not a retry, skip
  or timeout increase.
- Full corpus runs on frozen candidates and on claimed test hosts only.
- Findings grouped in the owning story; a new story only for independent
  ownership or acceptance.

## Operator-authorized scope split to Sprint 573

On 2026-11-05 the operator requested a new sprint for Windows, C# and
PowerShell, while Sprint 562 completes its remaining stories. Sprint 573
(UUID `a5f60348-0dc8-5af7-80aa-9dffe2c745ea`) owns those deferred platform
and language obligations, described in the umbrella document
`docs/plan-windows-csharp-powershell-followup.md`. Nine dedicated stories
retain their IDs and evidence there; carried parent `83fe3d72eb62` captures
the C#/PowerShell portion of E4. E4 `df328510d106` remains here for C/C++.

For current acceptance, the historical all-platform/all-language wording
above is amended only by this explicit split. Sprint 562 must finish its
remaining Linux corpora and non-deferred fence obligations, source-bound
regression gates and integration. C++, Rust, Python, Go and TypeScript
remain here. Bash's final serial gate remains required. Deferral never
counts as a pass; inventories and ratchets preserve the full denominators
and separately identify follow-up ownership. No unexplained skip, timeout
increase, weakened oracle or implicit waiver is permitted.
