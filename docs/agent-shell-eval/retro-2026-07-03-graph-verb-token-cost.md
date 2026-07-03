# Graph-Verb Token-Cost Retro - 2026-07-03

## Scope

A supporting diagnostic (non-agent, like the ShellBench/Koala batches) measuring
whether the newly shipped `graph-*` verbs (bashy v0.7.0: the code-knowledge graph
+ the writable contribution layer) **help** an agent or **stand in its way**. This
is the check the standard-benchmark-campaign retro template asks for — "did bashy
slow/confuse the agent?" — applied to a brand-new feature before it reaches a live
matrix.

- Method: direct measurement of verb output size (agent token cost) + the
  first-hop discovery surface (`bashy context`), on a real repo (`internal/cli`,
  ~155 graph nodes). No agent tools spent; no container matrix (the live container
  campaign is running separately — this avoids colliding with it).
- Build under test: `bin/bashy` at coreutils `03b23fe` → fixed.

## Finding: two verbs were agent-hostile by default (a token-bomb)

The value proposition of `graph-impact` is "skip the grep dance." Measured against
an actual grep for the same question ("what is coupled to `newRunner`?"):

| path | output |
| --- | --- |
| `grep -rn newRunner` | 8 precise lines |
| `graph-impact newRunner` (default) | **124 symbols / 28,257 bytes (~7K tokens)** |

The default `--depth 2` BFS on the **undirected** graph exploded (14 → 124 nodes),
so the "helpful" verb returned ~3,500× more bytes than grep for a simple question —
it would **cost** an agent tokens and bury the answer, not help. `graph-query` had
the same 28 KB explosion. The other verbs (neighbors/stats/hotspots) were already
lean (1–2 KB).

Second finding: the feature was **undiscoverable**. `bashy context --json` — the
first-hop record the campaign proved cuts ~31 tool calls by advertising
capabilities — did not mention the graph or knowledge-graph verbs at all. A
feature an agent can't discover can neither help nor be measured; it just sits as
latent surface.

## Fixes implemented (coreutils `cmds/graph` + bashy `context.go`)

1. **`graph-impact` default depth 2 → 1** (direct coupling = the first-order blast
   radius). Depth 2+ is opt-in via `--depth`.
2. **Bounded output with an explicit truncation signal** on `graph-impact` and
   `graph-query`: `--limit` (default 40), nearest-first (BFS order) so the cap
   keeps the most-impacted symbols, and `total`/`truncated` in JSON + a
   `… +N more` line in text — no silent caps.
3. **Capped results stay internally consistent** (`subgraphView`): only edges whose
   both endpoints survive the cap are emitted (no dangling edges).
4. **Discoverability:** `bashy context` now advertises `code_graph` +
   `knowledge_graph` capabilities and two recommended commands (`graph-impact`,
   `graph-recall`).

## Before / After

| Metric | Before | After | Delta |
| --- | ---: | ---: | ---: |
| `graph-impact` default output | 28,257 B (124 nodes) | 2,673 B (14 nodes) | **−91%** |
| `graph-impact --depth 2` (opt-in) | 28,257 B | 3,444 B (capped 40) | −88% |
| `graph-query` default output | 28,587 B | 2,854 B | **−90%** |
| `bashy context` graph discoverability | absent | 2 caps + 2 commands | now present |
| `bashy context --json` size | 1,420 B | 1,755 B | +335 B (+24%) |

## Retro answers

- **Did bashy help the agent?** After the fix, yes — `graph-impact` now returns a
  ~2.7 KB direct-coupling answer with structure grep can't give (relations,
  transitive reach on demand), at a token cost in grep's league. Before the fix it
  actively hurt.
- **Did bashy stand in the way?** Before: yes — a 28 KB default answer is a
  token-bomb, and the feature was invisible on the discovery path. Both fixed.
- **Defect class:** shell/feature-design defect (agent-hostile default), caught by
  a token-cost diagnostic rather than a pass/fail task.

## Follow-ups

- **Live agent A/B (recommended next campaign batch):** a code-navigation task
  ("list every symbol coupled to X and where it lives") with two arms — bashy
  (graph verbs advertised via `context`) vs GNU Bash 5.3 (grep only) — measuring
  tool calls / shell invocations / wall time. Hypothesis after the fix: bashy
  reduces navigation commands without a token blowup. Deferred here only to avoid
  colliding with the in-flight container campaign; the fix had to land first (an
  undiscoverable, token-bombing feature is not worth an agent run).
- **Directed-edge variant (F-D in `docs/graph-agentic-features-roadmap.md`):** would
  turn `graph-impact` from an undirected coupling neighborhood into true reverse-
  dependencies, shrinking the result further and sharpening precision.
