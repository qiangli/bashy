# bashy search — design

Status: design (P0b `pkg/search/local.go` shipped as a naive scan; this doc is
the architecture it grows into). Brand-neutral: bashy + coreutils only.

`bashy search` is the missing **find-things primitive** — the local half of a
two-layer stack whose web/research half is `bashy sota` (provider ladder →
SearXNG; out of scope here). It unifies what bashy already has —
`grep` · `find` · `ast` · `graph` · `kb` — behind one verb with a uniform
result, and it does so **without building a persistent code index.**

## The one load-bearing principle: no transient code index

> A scan is not an index. For code, a smart query + a fast scan + a cheap rank
> beat a full-text index — and cost nothing to keep current.

This is not an aesthetic preference; it falls out of what full-text search (FTS)
actually buys you over grep, and what it costs.

### What FTS has over grep — and why none of it needs an index *for code*

| FTS capability | Grep-side equivalent | Needs an index? |
|---|---|---|
| Stemming (`run`↔`running`) | **query expansion** → regex alternation | no |
| Fuzzy / typo (`recieve`↔`receive`) | query expansion (bounded edit distance) | no |
| Synonyms (`delete`↔`remove`) | query expansion (curated map) | no |
| Tokenization / word boundary | native (`\b`, `rg -w`) | no |
| Relevance ranking (BM25) | **post-hoc scoring** of the hits you found | no |
| Multi-term bag-of-words (A ∧ B) | grep A ∩ grep B (set intersection) | no |
| Postings lookup at massive scale | — (genuinely index-only) | yes, **but N/A** |

The lexical smarts are **query-expansion problems, not storage problems**: turn
the query into a regex before ripgrep sees it, and the "intelligence" lives in a
stateless function instead of a database. Ranking and multi-term are
**post-processing** over the hits already retrieved. The single capability that
truly requires an inverted index — O(postings) lookup across *millions* of
documents without touching them — **never activates for code**: a repo is
MB–GB, and ripgrep scans the whole thing in milliseconds (SIMD + parallelism +
mmap + gitignore pruning). You would pay a per-workspace build + GB of storage
to avoid a scan that finishes before you blink.

### Cautionary tale (why this is a rule, not a footnote)

A prior agentic coding harness carried a bleve full-text **code** index: ~27 GB
on a real workspace, a ~15-minute rebuild **per fresh workspace**, and its own
indexer would ENOSPC and stall the agent on a full disk. On inspection the grep
path *discarded* the index's candidate results and re-ran a full ripgrep anyway
— the index was queried and thrown away. It was removed; grep became ripgrep
only, with symbol/reference intelligence kept (that's what ripgrep can't do).
`bashy search` must never reintroduce that class of cost.

## Architecture: a router over live primitives

`bashy search` is a **router**, not a monolith and not a fan-out-by-default.
It classifies the query and dispatches to the cheapest primitive that can answer
it. (A `--all` fan-out mode exists for "find this anywhere," but it is opt-in —
fan-out is noisier and pays every lane's cost on every query.)

```
                         ┌──────────────┐
      query  ─────────►  │   classify   │
                         └──────┬───────┘
        ┌───────────────┬───────┼────────────┬──────────────┐
        ▼               ▼       ▼            ▼              ▼
   literal/regex     symbol   who-calls   concept/idea    filename
        │               │       │            │              │
     CONTENT          ast     graph /       kb            find
     pipeline      (symbols)  ast refs   (knowledge)    (name scan)
        │
   expand → scan(rg) → rank
```

- **Content** (`literal | regex | phrase | fuzzy`) → the expand→scan→rank
  pipeline below.
- **Symbol** ("where is `Foo` defined") → `ast` (treesitter, on-demand, no
  index).
- **Who-calls / impact** → `graph` / `ast refs`.
- **Concept / lesson** ("how do we handle retries") → `kb`.
- **Filename** → `find` / name scan.

All lanes return the **same uniform result** so the caller sees one shape.

## The content pipeline: expand → scan → rank

Three cheap, stateless stages. No persistent state between calls.

### 1. Expand — query → regex
A pure-Go function turns a term into a ripgrep pattern, applying only the
expansions the query mode asks for:

```
"run"      --stem-->     \brun(s|ning|ner)?\b|\bran\b
"recieve"  --fuzzy-->    rec[ei]{2}ve|receiv(e|ed|ing)
"delete"   --synonym-->  \b(delet|remov|drop|destroy|purge)\w*
```

Resources are a few KB of tables, not a database:
- a Porter-style stemmer (~one file),
- a bounded edit-distance variant generator (cap at distance 1–2),
- a curated, code-domain synonym map (`cfg`↔`config`, `impl`↔`implementation`, …).

**Caps are load-bearing:** an unbounded alternation kills ripgrep's fast path.
Bound the number of alternatives, prefer literal-heavy expansions (ripgrep's
Teddy / Aho-Corasick handles many literals fast), and fall back to the bare
query if expansion would explode.

### 2. Scan — ripgrep, not a hand-rolled walker
Delegate to the **coreutils grep engine** (`cmds/grep`), never a bespoke
substring scanner. Two reasons: every speedup to grep (RE2/Teddy, parallel
`fswalk`) then benefits search for free, and there is exactly one fast path to
optimize instead of two. (Today `pkg/search/local.go` hand-rolls a
single-threaded `filepath.WalkDir` + `strings.Contains` — that is the first
thing to replace: route to the grep engine.)

### 3. Rank — score the hits you already have
grep returns matches in file order; ranking is a post-pass over exactly those
hits, no index:
- **term rarity** — a hit on a rare token outranks a hit on a common one
  (compute rarity over the matched set, a poor-man's IDF),
- **match density** — files/regions with more, closer matches rank higher,
- **path priors** — source over vendored/generated (reuse the skip-dir list),
- **recency** (optional) — prefer recently-touched files.

## The prior/weight layer — differential search

A flat scan treats every file as equally likely to hold the answer. It isn't. A
**weight `w(file)`** — an *attention prior* over files — biases the content lane
toward likely-relevant files. This stays inside the no-index rule because **the
weights are metadata (which files, how much), never file content**: it is a
ranked-search-engine's smarts without its storage/build cost. The pipeline
becomes:

```
classify → weight(files) → priority-scan → weighted-rank → cluster
```

### The signals (composed into w(file), query-dependent)

| Signal | Source | Cost | Status |
|---|---|---|---|
| **type/role** — source ≫ test ≫ config ≫ docs ≫ generated/vendored | extension + path patterns | ~free | partial (skip-dirs) |
| **centrality** — hubs many files import | `graph hotspots` / in-degree | cheap | **exists** |
| **recency** — recently modified | git status + mtime | cheap | trivial |
| **working set** — files opened/edited this session | session access log | cheap | new |
| **hit history** — files that matched prior searches this session | search feeds itself | ~free | new |

The blend is **query-dependent** — the router picks the profile: a symbol search
weights source; a config-key search weights yaml/config; a "how do we handle X"
weights docs/kb. Same files, different priors per intent.

### How the weight is used (this is the "differential")

1. **Priority-ordered scan + early-exit** — feed files to the grep engine in
   weight order; once K solid hits come from high-prior files, stop. Effort is
   proportional to priority, not corpus size: a huge repo whose answer sits in
   the 5 hot files the agent already touched returns in ms. *(Requires a
   walk-order hook on the grep engine — see below.)*
2. **Weighted rank** — `final = match_score(rarity×density) × w(file)`.
3. **Cluster by category** — group results source/test/config/docs so the agent
   gets structured, not flat, output.

### Why this forces the pure-Go grep engine (not external ripgrep)

External ripgrep is a black box: its own walk order, scans everything, ranks
nothing, no early-exit hook. This layer *requires* controlling which files, in
what order, with early-exit, folding `w(file)` into the rank. The **coreutils
grep engine** exposes all of that because bashy owns the walker (today
`grep.go` walks `os.ReadDir`-sorted-by-name — that name-sort is the seam we
replace with a priority order). So "custom rg" = **grep engine + priority
walker + weighted ranker + session-signal store** — the differentiator no
off-the-shelf tool has.

### The session-signal store

A small per-session store: `path → {opened, edited, hitCount, lastSeen}` with
**time decay** (stale signals fade). Shape mirrors the space-time advisor's
host-ledger precedent — kilobytes, session-scoped, **never content**. Two feed
questions:
- **search-scoped vs agent-scoped working set** — only files the search
  surfaced, or every file the agent opened/edited (needs the harness to emit
  file-access events)? The latter is the stronger signal; it depends on
  tool-call telemetry.

### Decisions / tensions

1. **Cold start** — session 0 has no working-set/hits; fall back to static
   priors (type + recency + centrality). Signals accumulate *within* a task, so
   search sharpens as work proceeds. Graceful degradation.
2. **Prior orders, never excludes** — early-exit risks missing the lone hit in a
   low-prior file. The weight only *orders* the scan and *boosts* the rank; a
   low-prior file still surfaces if it is the only match. Keep an
   `--exhaustive` mode (scan all + rank) beside the default top-K-by-priority.
3. **Self-tuning, but decay** — hits feed the working set, so search learns the
   task; time-decay prevents a wrong early hit from biasing the rest of the
   session.

### Phasing note

Ship **static** priors first (type + centrality + recency — no session state,
most of the value), then layer the **learned** signals (working-set +
hit-feedback + decay) as P1.5. The learned part is the one that "feels magic,"
but the static part earns its keep on day one with zero state.

## Uniform result shape

```go
type Result struct {
    Kind  string // "content" | "file" | "symbol" | "ref" | "kb"
    Path  string
    Line  int     `json:",omitempty"`
    Text  string  `json:",omitempty"`
    Score float64 `json:",omitempty"` // ranked lanes only
}
```

`--json` emits this directly (token-lean, agent-facing); the human view renders
`path:line: text` grouped by score.

## What it delegates to (and must not reimplement)

- **grep engine** (`cmds/grep`) for the scan.
- **`ast`** (treesitter) for symbols/refs — on-demand, no index.
- **`graph`** for impact/neighbors.
- **`kb`** Go API for knowledge facts.
- **`find`** for name/glob.

`search` owns only: query classification, expansion, ranking, result merging.

## Non-goals / forbidden

- **No persistent code index at startup** — not FTS, not trigram, not symbols-
  pre-baked. The default path pays zero index cost.
- If profiling on real agentic workloads ever shows the scan is the bottleneck
  on genuinely huge repos, a cache (e.g. trigram candidate-filter) may be added
  **lazy + opt-in + measured** — built on first need, never a startup tax, never
  storing file content. Treat it as a last resort with evidence, not a default.
- **Web search is a separate layer** (`bashy sota` / provider ladder). `search`
  is local-only; conflating them re-imports network dependence into a
  local-first primitive.

## Phasing

- **P0 (mostly shipped, needs the router + grep delegation):** unify content +
  files + kb behind one verb with the uniform result. Replace the hand-rolled
  scan with the grep engine. Add the router classification (literal/symbol/
  who-calls/concept/name).
- **P1:** the expand stage (stemmer + bounded fuzzy + synonym map, all capped)
  and the rank stage (rarity × density × path prior). Wire the `ast`/`graph`
  lanes into the router. Add the **static** prior/weight signals (type +
  centrality via `graph hotspots` + recency) and the grep walk-order hook →
  priority-ordered scan with early-exit.
- **P1.5:** the **learned** prior signals — the session-signal store
  (working-set + hit-feedback + time decay). This is the "differential search"
  that sharpens within a task.
- **P2 (only if measured):** a lazy, opt-in trigram candidate cache for
  outlier-huge repos. Gate it like any index — never on by default.

## Open questions

1. **Router classification** — heuristic (cheap, predictable: regex-shape, a
   leading `sym:`/`ref:` hint, quoted phrase) vs a small LLM-assisted classifier
   (smarter intent, but adds a model call). Start heuristic; a `--kind` override
   always wins.
2. **Synonym source** — a curated code-domain map is small and high-signal;
   avoid a general thesaurus (noisy). Where does the map live and how is it
   extended (a `kb`-style contributable table)?
3. **Fan-out ranking** — when `--all` fans across lanes, how are cross-lane
   scores made comparable (content BM25-ish vs symbol exactness vs kb match)?

## SOTA validation & grounding (2026-07, via `bashy sota`)

Web-grounded research (8 sources, Brave-grounded) confirms this design sits where
the field actually is, and sharpens two points.

**The field converged where we did.** Production code search is a **two-stage
retrieve-then-rank** shape: a trigram index for candidate generation, then an
explicit **re-rank** over match quality, symbol-ness, file-kind, and repo/author
signals (Zoekt, GitHub Blackbird). The 2026 move for *agent* use is "grep's
replacement is three tools" — **lexical + structural (tree-sitter) + resolved
call-graph** — which is exactly this doc's **router over grep/ast/graph**. Every
weight signal here is a real production signal: GitHub ranks by **file-kind +
is-symbol**; Sourcegraph boosts **repos you recently contributed to** (our
recency/working-set prior); Cursor trains on **LLM-ranked relevance from real
sessions** (our learned working-set / hit-feedback). Sources:
`github.blog/…/new-code-search`, `sourcegraph.com/docs/code-search/features`,
`zzet.org/gortex/grep-replacement-for-ai-agents`.

**It's a crossover, not an absolute.** Zoekt/Sourcegraph/Blackbird index at
**billions of lines / millions of repos** — the index-only regime this doc
already concedes. A single repo is MB–GB, far below the crossover, where
ripgrep already scans in ms — so "no code index for single-repo" is correct
*for its stated scope*, not dogma. Index **freshness is a documented pain** (HN:
pushed a repo, still unindexed a day later), which is the flip side of our
always-current scan.

**Ranking is a *measured* win, not polish** — one (vendor, unreplicated) number:
BM25 R@5 55.1% vs ripgrep 17.3% at 3–50× fewer tokens [gortex]. Directional, but
it points at the rank layer as the agentic-cost lever, not the scan.

**The local single-repo ranking eval is a genuine gap.** The corpus is
thin-to-silent on exactly our priors (recency/centrality/working-set/query-
expansion for *local* search) — everyone published solves the *cross-repo scale*
problem. bashy would be first to ship (and could publish) that eval.

**Web-grounding corollary — rank the union.** An A/B of the *same* sota question
grounded on **Brave** vs a self-hosted **SearXNG** (added as the ladder's keyless
rung, `coreutils 7cdb7f8`): Brave returned **8/8 on-topic** (relevance-ranked
index) but vendor-skewed; SearXNG returned **3/8 on-topic + 5/8 noise** (raw
metasearch, unranked) yet surfaced **two unique on-topic sources Brave missed**
(Moderne "Trigrep", an AST-symbol MCP tool). Perf: SearXNG ~1.7 s/query + a
tens-of-seconds container start vs Brave ~0.85 s stateless. Lesson, one level up
from the local design: a relevance-ranked source beats raw metasearch for
*precision*; metasearch wins on *coverage*; so the right use is **merge
keyed ∪ SearXNG then dedupe + relevance-rank the union** — never concatenate raw.
The same expand→scan→**rank** shape, applied to web grounding. (SearXNG stays the
off-by-default second source; see `docs/licensing-supply-chain-policy.md` §2 for
the AGPL download+exec discipline.)

## References

- `pkg/search/local.go` — the shipped P0b scan this refines.
- `docs/bashy-agentic-performance-strategy.md` — the "fewer calls / faster /
  token-lean" levers this pipeline instantiates (`fswalk` parallel walk,
  RE2/Teddy grep, `--json`).
- `docs/coreutils-command-analysis.md` — the trigram-index note (here: deferred
  to P2, lazy-only, per the no-index principle).
