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
  lanes into the router.
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

## References

- `pkg/search/local.go` — the shipped P0b scan this refines.
- `docs/bashy-agentic-performance-strategy.md` — the "fewer calls / faster /
  token-lean" levers this pipeline instantiates (`fswalk` parallel walk,
  RE2/Teddy grep, `--json`).
- `docs/coreutils-command-analysis.md` — the trigram-index note (here: deferred
  to P2, lazy-only, per the no-index principle).
