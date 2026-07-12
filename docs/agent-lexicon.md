# The agent lexicon — teaching a fleet of tools your jargon

Status: **design (2026-07-12), SOTA-grounded.** No code yet.

## The problem

Mid-session, a user says:

> **"handoff this to codex."**

In this circle, neither word means what the dictionary says:

- **"handoff"** is not the English word. It is `bashy handoff` — a verb with precise semantics (capture the
  in-flight working tree, release the lease, dispatch).
- **"codex"** is not "OpenAI's product". It is *an agent binding on **this host***: a CLI tool plus a
  specific bound model, as registered in the fleet registry. On another machine the same word denotes a
  different binding.

This is jargon: inside the circle it needs no explanation, and outside it is meaningless or — worse —
plausibly wrong. The agent must resolve both referents **without the user re-explaining**, and it must
work across tools that will never agree on a config format.

## What the research says (July 2026)

**The problem is 20% glossary and 80% precedence + lookup.** Two findings reframed the design:

1. **A hand-written `GLOSSARY.md` is the known dead end.** It is stale the moment a host's registry
   changes. The entire data-catalog industry (DataHub, OpenMetadata, Collibra) exists because that
   failed. The mature operating model is: *the glossary **lives in the registry**, is **linked to the live
   assets it names**, and is **served to agents** — never hand-maintained.*

2. **Stuffing vocabulary into context actively harms grounding.** Tool-selection accuracy degrades past
   **~15–20 tools in active rotation**, and near-synonymous names are the top failure mode (Microsoft
   Research measured **775 tool-name collisions** across MCP servers; `search` collides across 32).
   *More vocabulary in context ≠ better resolution.*

**The strongest grounding primitive available is a generated `enum` in a tool schema.** A single tool

```jsonc
bashy_handoff(to: { "enum": ["codex", "claude", "opencode-glm4"] })   // ← generated from the fleet registry
```

makes "codex" a **schema token, not an English word**. The model structurally cannot name a binding that
does not exist on this host, and MCP's `tools/list_changed` re-derives the enum when the registry
changes. No prompt engineering, no synonym table. This is the mechanism; everything else is a fallback
for clients that cannot do MCP.

**`SKILL.md` is now genuinely cross-vendor** (Claude Code, Codex, OpenCode, Cursor, Gemini CLI, Copilot,
Goose, Amp — under Linux Foundation governance), and its spec explicitly says the `description` field
*"should include specific keywords that help agents identify relevant tasks."* That ~100-token,
always-in-context field **is the officially-sanctioned lexicon slot** in every tool we care about.

**`AGENTS.md` is the neutral always-on layer** (60k+ repos; read by Codex, Cursor, Aider, Copilot,
Gemini, Zed, Devin; Claude Code via `CLAUDE.md`). There is **no portable slash-command format** and
there never will be — do not build one.

### The 20-year-old prior art maps exactly

- **SKOS** gives the label model: `prefLabel` (`bashy handoff`), `altLabel` ("hand it off", "pass it
  to"), `hiddenLabel` (legacy names, misspellings), and — the killer field — **`scopeNote`**, which is
  *precisely* the slot for the hard part: *"within this workspace, 'handoff' never means the English
  word."*
- **TBX (ISO 30042)** gives **concept-orientation**, and it dissolves the dynamic-referent problem
  outright: the **concept** is *an agent binding (tool × model, host-scoped)*; **"codex" is merely a
  term that denotes it on this host.** Term ≠ concept is the whole trick.
- **DDD's ubiquitous language**, restated for 2026: it *"must now be machine-readable, constraining how
  an LLM interprets words within a bounded context."* **The repo/host IS the bounded context.**

## What bashy already has, and never projects

This is the key realisation: **the term base already exists. It has simply never been shown to anyone as
vocabulary.**

| half of the lexicon | where it already lives | shape |
|---|---|---|
| **verbs** (`handoff`, `weave`, `gate`, `meet`, `sprint`) | the **Command Atlas** (`coreutils/pkg/atlas`) | name + synopsis + SDLC stage + group + effects — curated, closed-vocabulary, ratcheted at init |
| **agent bindings** (`codex`, `claude`, `opencode-glm4`) | the **fleet registry** (`bashy agents` — "named `tool:model` bindings, the enlistable unit") | live, host-specific, already a registry |
| **capabilities** (`conductor`, `knowledge-transfer`) | **`bashy skills`** | already `SKILL.md`, already exported to `.claude/skills` and `.agents/skills` |

Three registries, generated and maintained. Zero projection into the channels an agent actually reads.

## The sigil already exists, and it is the word `bashy`

*(User, 2026-07-12 — and it is a better answer than inventing punctuation.)*

> **"bashy handoff this to codex"**

No new syntax, nothing to teach, nothing to remember. The word that already names the tool declares that
**everything after it is bashy vocabulary**. It is the natural disambiguator, and it was there all along.

Pair it with the **enum** and the ambiguity disappears entirely: `handoff --to` accepts only the agent
tools that exist on *this* host — `codex | claude | opencode | agy | aider`. In that position "codex"
**cannot** mean OpenAI's product, because **that is not a legal value**. The word is grounded *by
construction*, not by persuasion:

```
$ bashy handoff --to codex   →  handing to codex-gpt-5.5
$ bashy handoff --to gpt5    →  "gpt5" is not an agent on this host.
                                Valid: agy, aider, claude, codex, opencode
```

A closed value set grounds a word harder than any description can — and unlike a glossary it **cannot go
stale, because it IS the registry**. This is the same mechanism the research named as the strongest
available (a generated `enum` in a tool schema); the insight is that bashy can have it *today*, at the
CLI, without waiting for an MCP server.

**So there are three levels of marking, in increasing explicitness — and only the first is required:**

| level | form | when |
|---|---|---|
| **plain** | `handoff this to codex` | inside the circle. Resolved by the precedence rule + the lexicon. |
| **prefixed** | `bashy handoff this to codex` | when the context is ambiguous. **`bashy` is the sigil.** |
| **marked** | `[[handoff]] this to [[codex]]` | in written artifacts, and when a human must force a resolution. |

## The marker: `[[term]]` — and where it belongs

A term needs to **stand out**, the way `/` marks a slash command as "not a plain word". That instinct is
right. But the *location* of the marker is the whole design, and getting it wrong reinvents the problem.

### A sigil in the conversation defeats the purpose

If the user must type `/handoff this to /codex`, we have rebuilt slash commands: per-tool, non-portable,
and requiring the human to remember an exact syntax. Worse, it breaks the very thing that makes jargon
*jargon* — **inside the circle it is used unmarked.** Nobody in a standup says "slash-handoff". The point
is that you say "handoff this to codex" and everyone simply gets it.

### The marker belongs in the ARTIFACTS

> **Marked in writing → recognised unmarked in speech.**

This is how humans learn jargon: you see it **defined and emphasised** in the onboarding doc, then you
hear it plain in the standup. The marker's job is not to trigger a command. Its job is to **teach the
term set** and to make every mention **machine-detectable** in the corpus an agent already ingests —
skills, kb pages, project docs, handoff briefs, issue bodies, `CLAUDE.md`/`AGENTS.md`.

### The notation is `[[term]]`, and it already exists here — do not invent one

| why | |
|---|---|
| **already the convention in this ecosystem** | the kb roadmap specifies `[[wikilinks]]`; the agent memory system already links with `[[name]]` |
| **LLMs have seen millions of examples** | MediaWiki, Obsidian, Roam — recognition is free, no teaching required |
| **trivially detectable, renders in Markdown, namespaces cleanly** | `[[handoff]]` · `[[agent:codex]]` · `[[verb:gate]]` |

> ⚠️ **A claim I made and the scanner falsified on its first run.** I wrote that `[[ ]]` has *"zero
> collision with shell syntax"*. That is true of **bash parsing prose** — and **false of a shell
> project's DOCS**, which are full of bash:
>
> ```
> [[:alpha:]]      a POSIX character class
> [[ -z "$x" ]]    a test expression
> [[match]]        a TOML section header in an example
> ```
>
> `bashy lexicon scan` matched all three on its first run. The marker survives, but the token had to be
> tightened (letter-initial, no spaces, at most one namespace) **and code has to be stripped before
> scanning** — a `[[term]]` inside a code block is a code sample, not a mention. In a project whose
> subject matter *is* the shell, that distinction is the difference between a useful scan and noise.
>
> This is exactly why the marker's falsifiability matters: the tool caught its own author's error before
> a human did.

### The loop

| where | form | job |
|---|---|---|
| **artifacts** (docs, skills, kb, briefs, issues) | `[[handoff]]`, `[[codex]]` | **teach** the term set; make mentions detectable; link to the definition |
| **always-on tier** (`AGENTS.md`, `context --json`) | the precedence sentence + ~15 terms + the resolver | **seed** the agent's working memory on the first hop |
| **conversation** | plain `handoff this to codex` | **use** it — unmarked, like a human |
| **when unsure** | `bashy lexicon resolve handoff` | **look it up** — a name is resolved, never memorised |

And the marker remains available as an **escape hatch**: if an agent mis-resolves, a human can
disambiguate by writing `[[handoff]] this to [[codex]]`. **Optional emphasis, never required syntax** —
strictly better than a sigil you are obliged to type.

### What the marker buys, concretely

1. **Onboarding without a lecture.** `bashy lexicon scan` can walk the project's artifacts, collect every
   `[[term]]`, and check it against the registries — so the term set is *derived from how the team
   actually writes*, not from a list someone maintains.
2. **A falsifiable glossary.** A `[[term]]` that resolves to nothing is a broken link, and broken links
   are *findable*. A plain-prose glossary can rot silently; a linked one cannot.
3. **Definition-at-point.** In any artifact an agent reads, the marked term can carry its own resolution
   — the kb page, the atlas entry, the binding — so the agent does not have to guess *or* look up.
4. **It composes with what already exists.** kb pages, memory links, and skill bodies all speak this
   notation already. The lexicon is not a new syntax; it is the same one, pointed at the registries.

## Cross-tool validation (2026-07-12) — it works, and the test earned a fix

The only test that counts: does a **cold session of a foreign tool**, reading nothing but `AGENTS.md`,
resolve the jargon? Asked of `codex` and `opencode`, in this repo, with no other context:

> *Someone says to you: "handoff this to claude". In THIS project, what exactly do they mean?*

**Round 1 — the meaning landed; the syntax did not.**

Both tools resolved the *hard* part perfectly. Neither mistook "claude" for Anthropic's product:

| | codex | opencode |
|---|---|---|
| "claude" | *"the local Claude CLI binding: `claude-fable` or `claude-opus`"* ✅ | *"The `claude` CLI **on this host**, bound to claude-fable (fable) or claude-opus (opus)"* ✅ |
| "handoff" | *"pause this session and transfer the work to another agent, scheduler, or future session"* ✅ | ✅ |
| **command** | `bashy handoff claude` ❌ | `handoff claude` ❌ |

**Both guessed the invocation wrong, and in the same way** — no `--to`. The block taught **meaning** but
not **usage**, so each had to invent the flag. A vocabulary that tells an agent what a word *means* but
not how to *say* it has done half a job.

**The fix was two lines** — teach the sentence, not just the words:

```
The sentence this vocabulary exists for, and its exact form:

    "handoff this to codex"   →   bashy handoff --to codex -m "<why, for the successor>"
    "resume it"                →   bashy resume
```

**Round 2 — both exact:**

| | codex | opencode |
|---|---|---|
| **command** | `bashy handoff --to claude -m "<why, for the successor>"` ✅ | `bashy handoff --to claude -m "<why, for the successor>"` ✅ |

Two lines of always-on context, and two foreign tools now speak the project's jargon correctly on their
first breath. **That is the whole feature, validated.**

## Design: one term store, three projections

**Do not write a glossary. Project the registries you already keep.**

### The store (M4) — SKOS/TBX-shaped, mostly generated

```
concept:   verb:handoff                       │  concept:  binding:codex@this-host
prefLabel: "bashy handoff"                    │  prefLabel:"codex"
altLabel:  ["handoff", "hand it off",         │  altLabel: ["codex cli"]
            "pass it to"]                     │  definition: generated — tool=codex, model=…
definition: (from the atlas synopsis)         │  scopeNote: "an agent binding ON THIS HOST,
scopeNote: "In this workspace this is the     │              not a product. Resolve, never assume."
            bashy verb, never the English     │  status:    live (from the fleet registry)
            word."                            │
```

**Generated:** every concept, `prefLabel`, `definition`, and status — from the atlas and the fleet
registry. **Hand-written:** only `altLabel` (the colloquialisms a team actually says) and `scopeNote`
(the precedence rule). That ratio is the point: the parts that go stale are generated; the parts that
need judgment are tiny.

### Projection 1 — a generated `enum` in a tool schema *(hard grounding; the mechanism)*

One MCP tool per verb with a `to:`/`agent:` parameter whose `enum` is generated from the fleet registry,
refreshed via `tools/list_changed`. **Name it `bashy_handoff`, not `handoff`** — MCP has no formal
namespace, the spec's disambiguation guidance is only SHOULD-level, and a distinct name does more
grounding work than any description. Expose **few** tools (stay under the ~15–20 degradation cliff):
prefer one `bashy_run(verb: enum[…])` over twenty per-verb tools.

### Projection 2 — a managed block in `AGENTS.md` *(always-on, universal)*

`bashy lexicon --emit agents-md` writes a **fenced, regenerated** block:

- **the precedence rule, in one sentence** — *"In this workspace, these words are bashy verbs and agent
  bindings, never their English senses."*
- ~15 highest-value terms (not the whole registry — see the degradation cliff)
- **the resolver command**: `bashy lexicon resolve <term> --json`

Generated, never hand-written. Short, because this tier is paid for on **every turn**.

### Projection 3 — a `bashy-lexicon` skill *(on-demand, cross-vendor, ~100 tokens resident)*

A `SKILL.md` whose **`description` carries the trigger words** ("handoff, resume, weave, gate, sprint,
meet, conductor, codex, the fleet — in this workspace these are bashy verbs and agent bindings, not
English words") and whose **body delegates resolution to the live registry** rather than restating it.

That split is the whole architecture: **the description does mention-detection; the body does entity
resolution against the registry.** It is the only way a *dynamic* vocabulary fits in a *static*
always-on tier — and it is exactly what progressive disclosure is for.

## The resolver is the 80%

```
$ bashy lexicon resolve codex --json
{ "term": "codex", "concept": "binding:codex",
  "kind": "agent-binding", "host": "dragon-2.local",
  "tool": "codex", "model": "…", "status": "live",
  "scope_note": "an agent binding ON THIS HOST, not a product",
  "use": "bashy handoff --to codex" }
```

**A name is resolved by a lookup, never memorised.** That is the one sentence worth stealing from the
agent-naming stack (A2A Agent Cards, ANS, DNS-AID): a card is published by the agent, collected by a
registry, and *looked up* — never baked into a prompt.

## What NOT to build

- ❌ **A required sigil in the prompt** (`/handoff`, `@codex`). It rebuilds slash commands — per-tool,
  non-portable — and it breaks the property that *makes* jargon useful: inside the circle, it is used
  **unmarked**. The marker belongs in the artifacts that TEACH the term, not in the sentence that USES
  it. Keep `[[term]]` available as optional emphasis; never require it.
- ❌ **A hand-written glossary file.** Stale on the first registry change. If it is not generated, do not
  ship it.
- ❌ **The whole registry in context** — as prose or as N tools. It *degrades* grounding.
- ❌ **A portable slash-command abstraction.** `.claude/commands/` vs Codex prompts vs Cursor rules vs
  aider `/cmd` are irreconcilable, and skills already subsumed them.
- ❌ **MCP `resources` as the vocabulary channel.** Application-controlled by spec; most clients never
  auto-inject them. Structurally cannot be an always-on tier.
- ❌ **Load-bearing semantics in optional `SKILL.md` frontmatter** (`allowed-tools`, `paths`) — silently
  ignored by most clients. Only `name`, `description` and the body portably exist.

## Anti-bloat check (the atlas asks this of every new verb)

> *Which SDLC stage does `lexicon` serve that nothing else already does?*

**`cross`.** And what it does that nothing else does: it **projects** three existing registries (atlas,
fleet, skills) into the three channels agents actually read. It introduces **no new source of truth** —
which is the test it has to pass. If it ever starts *storing* vocabulary rather than *projecting* it, it
has become the hand-written glossary we said not to build.

It also **merges with an existing obligation**: the coherence pass already owes a rewrite of
`bashy/AGENTS.md` (a 4-line stub today, read **first** by several tools) and `coreutils/AGENTS.md` (a
divergent doc that never mentions the contract). The lexicon block is the *content* that rewrite was
missing.

## Sources

MCP tools spec + `tools/list_changed`; Microsoft Research, *Tool-Space Interference in the MCP Era* (775
name collisions); Agent Skills spec + client showcase (agentskills.io, Linux Foundation AAIF);
agents.md; W3C SKOS; ISO 30042 (TBX); DataHub/OpenMetadata MCP glossary servers; A2A Agent Cards, ANS
(arXiv 2505.10609), DNS-AID; *Tool-to-Agent Retrieval* (arXiv 2511.01854).
