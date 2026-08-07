# External-benchmark composite method

*Drafted 2026-08-06 (weave issue-42). Status: **proposal / method of record for the
external-prior layer**. Changes no production routing code. Companion:
`external-benchmark-capability-matrix.md` (the data), `band-ladder.md` (the host
ladder this feeds), `agent-bands-and-nicknames.md` (identity rules).*

*Note: the plan referenced as `../docs/agent-leaderboard-and-role-requirements-plan.md`
was not present in this workspace at drafting time; this method was derived from the
two in-repo band documents and the fleet registry
(`coreutils/pkg/fleet/baseline/{models,agents}`).*

---

## 1. What this is, and what it is not

Bashy's band ladder is deliberately honest about its epistemics: `declared` <
`operator` < `measured`, and a `~` on everything unmeasured. External public
benchmarks (SWE-bench Verified, Terminal-Bench, τ²-bench, OSWorld, METR horizons, …)
are a fourth kind of evidence: **nobody on this host ran them, but somebody
reputable did, at a sample size no host bake-off will ever match.**

This document defines how external benchmark results become a **prior** — a
per-capability score with provenance and uncertainty — for the exact canonical
`tool:model` bindings in the fleet. Three ground rules, inherited from the
registry's own discipline:

1. **An external prior is never a measurement.** It can never set
   `band_source: measured`, and it never overrides a host-measured result (the
   glm-5.2 / kimi-k3 "cannot converge" findings were invisible to every public
   leaderboard). It ranks with, and is distinguishable from, `declared`.
2. **No score is ever fabricated or interpolated.** A model absent from a source
   is `missing`, not 0, not the family average, not the nearest sibling. Missing
   cells are first-class and reported.
3. **Raw rows are kept.** Every composite must be recomputable from the raw
   source rows stored beside it. A composite whose inputs are gone is retired,
   not trusted.

Out of scope: changing the router, changing `quality:`/`band:` fields, or any
automated write into `coreutils/pkg/fleet/baseline/`. The output of this method is
a document + machine-readable file that a human (or a gated future job) reads when
re-pegging.

## 2. Identity: what a row may attach to

### 2.1 Three layers of identity

| layer | example | what external data attaches to it |
|---|---|---|
| **model** (canonical, version-explicit) | `opus4.8` → `claude-opus-4-8` | benchmark rows for that exact public model version |
| **binding** (`tool:model`) | `claude:opus4.8` | rows where the *harness is also comparable* (e.g. Terminal-Bench lists agent+model pairs) |
| **composite binding** (cascade) | `ycode-cascade-x4` | **nothing directly** — no public benchmark runs this composite; its prior is `derived` from constituents and must be labeled so |

External benchmarks almost always score a *model under some scaffold*. The
scaffold is part of the result. So:

- A row attaches at the **binding** layer only when the benchmark's scaffold is
  the same tool the binding uses (e.g. a Terminal-Bench entry running Codex CLI
  with `gpt-5.x` informs `codex:gpt5.x` directly).
- Otherwise it attaches at the **model** layer and *every* binding of that model
  inherits it, discounted by a **harness-transfer factor** (§6.3) — because the
  fleet has measured, repeatedly, that the harness changes the outcome
  (deepseek-v4-pro looked incapable under a buggy ycode stream handler;
  gemini3.1's demotion is confounded by agy).

### 2.2 Name mapping is evidence, not vibes

For each canonical model, the matrix carries an explicit `external_ids` mapping
(e.g. `opus4.8` ↔ the exact string a leaderboard prints). A row may only be
ingested when the source string maps to a canonical model **exactly and
unambiguously** — same version, same variant, same reasoning-effort tier where
the vendor distinguishes tiers. "Gemini 3.x Flash" does not inform
`gemini3.5-flash-low` (a different effort tier) unless the source says which
tier it ran. Ambiguous mappings are recorded in the matrix's *unmapped rows*
section rather than guessed. A fleet model with no confirmed public identity has
an **empty row of `missing` cells — that is a result**, and it is the honest one.

## 3. Capability axes and the benchmark map

The eight role-relevant capabilities, and the primary public instruments for each
(versions as of the 2026-08-06 retrieval; the matrix records what was actually
retrievable):

| capability | maps to fleet role | primary instruments | notes |
|---|---|---|---|
| `coding` (bug fixing / implementation) | L2 coding | SWE-bench Verified; SWE-bench Pro; Aider polyglot; LiveCodeBench | SWE-bench Verified is saturating at the top; Pro and polyglot keep spread |
| `terminal` (shell) | all bands (bashy IS the shell) | Terminal-Bench (agent+model leaderboard) | closest external proxy for "drives bashy well" |
| `tool_use` | L3 conductor floor | τ-bench / τ²-bench; BFCL | pass^k on τ² is the only public *reliability-under-repetition* metric |
| `planning_orchestration` | L3 conductor | **no adequate direct public instrument** — proxied by τ²-bench, long-horizon evals | host repeat-ratio metric stays authoritative (§7) |
| `code_review` | L1 QA verify / review lanes | **no reputable dedicated public benchmark** | gap; host evidence only (glm-5.2's measured review strength has no external counterpart) |
| `browser_computer_use` | research/browse lanes | OSWorld(-Verified); Online-Mind2Web / WebArena lineage | scaffold-dominated; transfer factor low |
| `research` | steward support, deep-research | HLE (with tools); BrowseComp; GAIA lineage; DeepResearch-style evals | tool-augmented scores only comparable within a harness |
| `long_horizon` | conductor/steward reliability | METR 50% time horizon; Vending-Bench-style coherence evals | METR publishes CIs — rare and valuable |

A capability with no direct instrument is **reported as proxied or missing**, not
silently backfilled from a neighboring capability.

## 4. Normalization

Each benchmark metric is mapped to a normalized score `s ∈ [0,1]` **within one
(benchmark, version, split, metric) cell** — never across benchmarks or versions:

- **Pass/success rates** (SWE-bench %resolved, OSWorld success, τ² pass^1):
  `s = rate` (already 0–1). No re-anchoring: rates are absolute and comparable
  within the benchmark by construction.
- **pass^k reliability** (τ²): keep `k` in the metric name (`pass^4` is a
  different column from `pass^1`); never average across k.
- **Elo / preference ratings** (arena-style): logistic-transform against a fixed
  published anchor pair: `s = 1 / (1 + 10^((anchor − elo)/400))`. Anchor and its
  date are recorded in the matrix header; when the anchor moves, the algorithm
  version bumps (§9). Preference Elo is capped at weight `q ≤ 0.5` (§6.1) — it
  measures preference, not task completion.
- **Time horizons** (METR 50% horizon): `s = clamp(log2(h / 1min) / log2(H_ref /
  1min), 0, 1)` with `H_ref` a fixed reference horizon recorded in the matrix
  header (log-scale because horizon doubling, not linear gain, is the observed
  growth law). Report the raw horizon beside the normalized value always.
- **Unbounded/ordinal metrics** with no principled transform: left raw and
  excluded from composites; they appear in raw rows only.

Never min-max normalize against "the models that happened to be on the page" —
that makes a model's score change when a *different* model is added to a
leaderboard.

## 5. Provenance classes

Every raw row carries exactly one provenance class:

| class | meaning | examples |
|---|---|---|
| `independent` | maintainer or third party ran the eval themselves | Terminal-Bench leaderboard entries, Aider polyglot, Epoch AI reproductions, METR |
| `vendor` | number appears only in the model vendor's own card/blog | launch-post benchmark tables |
| `vendor-on-independent-harness` | vendor ran, but on a pinned public harness with published config | many SWE-bench Verified claims |
| `press/secondary` | media or aggregator citing one of the above | **never ingested as a row** — at most a pointer to find the primary |

Vendor rows are ingested (they are often the *only* row for a new model) but are
(a) weight-discounted (§6.1), (b) rendered with a `ᵛ` marker in the matrix, and
(c) a composite resting *solely* on vendor rows is flagged `vendor-only` at the
composite level — it can inform a `declared` band, never argue with a host run.

## 6. The composite

### 6.1 Per-row weight

For raw row *i* of a capability cell:

```
w_i = q_i × r_i × v_i × ind_i × t_i
```

- **q_i — source quality** ∈ {1.0 curated leaderboard/paper with published
  harness+n; 0.7 published eval, partial methodology; 0.5 preference/arena; 0.3
  self-published without methodology}.
- **r_i — recency**: `exp(−age_days / 180)` half-life ≈ 4 months, floor 0.2.
  Age counts from the *result's* date, not retrieval date. Benchmarks decay for
  a reason: contamination accumulates and harnesses improve.
- **v_i — version penalty**: 1.0 for the benchmark's current major version, 0.5
  one major version back, 0 older (an old version is a different test).
- **ind_i — independence**: 1.0 `independent`, 0.8 `vendor-on-independent-harness`,
  0.5 `vendor`.
- **t_i — relevance/transfer**: 1.0 when the row attaches at the binding layer
  (harness matches); otherwise the capability's harness-transfer factor —
  default 0.8 for terminal/coding/tool-use (scaffold matters), 0.6 for
  browser/computer-use (scaffold dominates), 1.0 for model-intrinsic evals run
  without scaffold.

### 6.2 Correlated sources

Rows are grouped into **lineage clusters** before aggregation: rows that are the
same underlying run (a vendor number echoed by a leaderboard; a leaderboard
mirrored by an aggregator; two splits of the same eval family scored by the same
maintainer) share a cluster. Within a cluster only the best-provenance row
survives; a cluster contributes **one** effective row. This is what stops one
vendor launch-table, syndicated five times, from posing as five agreeing
sources. Cluster ids are recorded in the raw rows so the collapse is auditable.

### 6.3 Aggregation

Per capability cell (model × capability):

- **n_eff = Σw_i** over surviving cluster representatives.
- **Composite = weighted median** of normalized scores (robust to a single
  outlier source; with ≤2 rows it degenerates to the weighted mean, and the cell
  is marked low-confidence).
- **Disagreement = weighted MAD**; reported beside the composite.
- **Confidence grade**:
  - **A**: ≥3 independent clusters, MAD ≤ 0.05, ≥1 row with published CI
  - **B**: ≥2 clusters, at least one independent
  - **C**: 1 cluster, independent
  - **D**: vendor-only (any count) — usable as a `declared`-grade prior only
  - **—**: no rows → cell is `missing`; no composite exists

### 6.4 Uncertainty

Where a source publishes CIs (METR does; most leaderboards do not), the CI is
carried on the raw row and the composite reports the widest surviving CI rather
than pretending precision. Where no CI exists, uncertainty is expressed only
through grade + MAD + n_eff — **never as an invented ±**.

### 6.5 Missing data

- A missing cell is `missing`. It never averages as zero, and a role composite
  over capabilities (if anyone builds one) must skip-and-disclose, not impute.
- A model with < 2 graded cells across all eight capabilities gets **no roster
  row at all** in any derived ranking — an empty prior must not rank above or
  below anything; it is simply absent, and listed in the coverage report.
- Coverage is a first-class output: every matrix publication states
  `cells_filled / cells_total` per model and per capability.

## 7. Blending with host evidence

External priors and host evidence occupy different rungs and **the host rung
wins**:

```
measured (host, gated ladder run)        — authoritative; external NEVER argues
operator (host, lived runs)              — outranks external on the axes it observed
external prior (this document)           — informs declared pegs; grade A/B only
declared (vendor tier + priors)          — the floor external priors improve on
```

Concretely:

- glm-5.2 is `measured` L2 with a *measured inability to converge*. No public
  coding score — however high — touches that. The external prior may still note
  "external coding evidence is strong," which is exactly the useful tension: it
  says the model is worth routing *coding* work to, which the registry already
  concluded from the same host runs.
- For an unmeasured model (`gpt5.6-terra`, everything `declared`), a grade-A/B
  external prior is the **best available evidence** and should be cited in the
  yaml comment when a band is declared or a `quality:` prior is set — with the
  matrix row's id, so the citation is checkable.
- Numerical blend, when a single number is wanted for a *prior* (never for a
  band): `prior = (h·host + n_eff·ext) / (h + n_eff)` where `host` is a
  host-evidence score on the same 0–1 scale and `h` counts gated host runs ×2
  (a host run on *this* workload is worth more than an external row, sample size
  notwithstanding). With h=0 this reduces to the external composite; as host
  evidence accumulates it dominates. Grade-D (vendor-only) external cells enter
  with n_eff × 0.5.

## 8. Reproducibility and audit

- Raw rows (source URL, retrieval date, exact model string, metric, value,
  provenance class, cluster id) live in the matrix doc and its machine-readable
  mirror. Composites are pure functions of those rows plus this document's
  constants.
- Every published composite carries `algo: ebc-v<semver>` (§9) and the retrieval
  date. Recomputing with the same rows and algo version must reproduce the
  number bit-for-bit; if it cannot, the composite is invalid.
- Sources that were unreachable at retrieval time are listed as such — an
  unreachable leaderboard is recorded absence, not silent absence. (Absence of
  evidence is this codebase's characteristic failure; see
  `absence-of-evidence.md`.)

## 9. Algorithm versioning

`ebc-v1.0.0` is this document. Bump:

- **major** — any change that can reorder models given identical rows (weights,
  normalization, anchor moves, aggregation function);
- **minor** — new capability axis, new benchmark admitted to the map;
- **patch** — prose, transfer-factor of a *new* benchmark, source added.

A matrix file states the algo version it was computed under; mixing versions in
one table is forbidden.

## 10. Machine-readable schema (proposal)

Proposed file: `docs/external-priors.yaml` (or later
`coreutils/pkg/fleet/external/`), schema id `bashy-external-prior-v1`. Not
consumed by any production code yet — the schema exists so the matrix is
mechanically checkable before anything consumes it.

```yaml
schema: bashy-external-prior-v1
algo: ebc-v1.0.0
retrieved: "2026-08-06"
anchors:
  elo_anchor: {source: "<arena>", value: 1400, date: "2026-08-06"}
  metr_h_ref: "8h"
sources:
  - id: swebench-verified
    url: https://www.swebench.com/
    version: "verified-500"
    metric: pct_resolved
    quality: 1.0
    kind: independent            # default provenance of rows from this source
rows:                            # RAW rows — the reproducibility substrate
  - source: swebench-verified
    model_external: "<exact string the source printed>"
    model: opus4.8               # canonical fleet name, or null if unmapped
    capability: coding
    value: 0.0                   # as printed (units per source metric)
    normalized: 0.0
    result_date: "2026-XX-XX"
    retrieved: "2026-08-06"
    provenance: independent | vendor | vendor-on-independent-harness
    harness: "<scaffold as stated>"
    n: 500
    ci: null                     # or {lo: , hi: , level: 0.95}
    cluster: "<lineage cluster id>"
    note: ""
composites:
  - model: opus4.8
    capability: coding
    value: 0.0
    grade: A | B | C | D
    n_eff: 0.0
    mad: 0.0
    vendor_only: false
    inputs: [<row indexes>]      # composite must be recomputable from these
coverage:
  cells_total: 0
  cells_filled: 0
  models_without_evidence: []
unmapped_rows: []                # source strings that matched no canonical model
unreachable_sources: []
```

Validation rules a checker should enforce: every composite's `inputs` exist and
share its capability; no composite without rows; `model: null` rows never feed a
composite; grades follow §6.3 mechanically; every `vendor_only: true` composite
has only vendor/vendor-on-independent rows.

## 11. Refresh triggers

The matrix is a snapshot; these events obligate a refresh (re-retrieval + a new
dated matrix file, old one kept):

1. **A fleet registry change** — model added/re-pegged (`baseline/models/` diff)
   → refresh that model's rows before citing any prior in the peg comment.
2. **A vendor ships a model the fleet binds** (family alias re-points, e.g.
   `opus` → opus5 on 2026-07-24) → within a week.
3. **A benchmark majors** (SWE-bench version, Terminal-Bench 2.x → 3.x,
   τ-bench → τ²) → version penalty zeroes old rows; refresh or the capability
   goes dark.
4. **Staleness clock** — any capability whose newest surviving row is > 180 days
   old (the recency half-life) is flagged `stale` in the next publication;
   > 365 days, its composites are withdrawn (rows kept).
5. **A host measurement contradicts a grade-A external prior** by more than 0.2
   normalized — investigate the *mapping and harness transfer* first (the
   deepseek stream-stall taught that the harness is the usual culprit), then
   re-retrieve.
6. **Quarterly** at minimum, even if nothing above fired.

Each refresh appends to a changelog in the matrix doc: date, trigger, algo
version, cells changed.

## 12. Known limitations (stated, not hidden)

- **Contamination / teaching-to-the-test**: public benchmarks leak into training
  corpora; recency decay and version penalties mitigate, not solve. Host gates
  remain the ground truth for "works here."
- **Scaffold entanglement**: most agentic numbers are (harness × model); the
  transfer factor is a declared constant, not a measured quantity — measuring it
  (same model, two harnesses, same task) is exactly the experiment the
  band-ladder doc already prescribes for gemini3.1.
- **Selection bias in coverage**: vendors benchmark where they win; independent
  leaderboards cover models their maintainers can access. Missing cells are not
  random, and the coverage report is there so nobody mistakes coverage for
  capability.
- **Anchor bias**: the fleet's band anchor is Anthropic-centric (disclosed in
  `band-ladder.md`); external priors inherit whatever anchor bias their sources
  carry.
