# knowledge-transfer — reference

Deep companion to `SKILL.md`. Read this before your first transfer campaign.

## Why this exists

Agent fleets have the same knowledge problem as human teams: experience
accumulates in individuals (one tool's private memory, one session's context)
and evaporates when the individual rotates out. Real teams solve it with a
shared wiki plus a discipline — seniors distill what they learned, juniors
read before doing, and claims graduate from "someone said" to "the team
confirmed" only through use. `bashy kb` is that wiki for every agent on a
host; this skill is that discipline.

The kb's own design supplies the machinery the discipline needs:
reconcile-on-write (no blind appends), supersede-not-delete (corrections stay
linked), the candidate → validated evidence ladder, journal provenance, and
deterministic search. The transfer process is deliberately NOT a workflow
engine: judgment (what to transfer, how to compress) belongs to the agent;
the CLI verbs supply ground truth and structure.

## Source-store reference

| Store | Path | Format | Ownership rule |
|---|---|---|---|
| Claude Code private memory | `~/.claude/projects/<project-id>/memory/*.md` (+ `MEMORY.md` index) | frontmatter markdown, one fact per file | read-only to everyone but Claude Code |
| ycode memex (global) | `~/.agents/ycode/memory/*.md` | frontmatter markdown; fields incl. `Importance`, `ValidFrom/Until`, `SupersededBy`, `Tags` | read-only to everyone but ycode |
| ycode memex (project) | `<repo>/.agents/ycode/memory/*.md` | same | same |
| weave campaign memory | `~/.bashy/weave/<tag>/memory.jsonl` | JSONL `Observation` records (issue, tool, outcome, summary, failed_approaches) | append-only via weave; transfer reads only |
| repo graph contributions | `<repo>/.agents/bashy/graph/contrib.jsonl` | JSONL note/link/observe records with provenance | append-only via `bashy graph-*`; transfer reads only |
| in-context recall | (the running session) | — | the mentor's own judgment |

The hard boundary, worth restating: **the kb reads foreign stores; it never
writes them.** Every store above belongs to the tool that maintains it.
`kb search --federate` already bridges the two repo-ring stores read-only —
transfer extends the same stance to the private-memory stores.

Notes per source:

- **Frontmatter-md stores (Claude, memex)** look deceptively kb-shaped. They
  are NOT kb pages: they are self-referential ("I", "this session"), often
  hostname-laden, scoped to one tool's workflow, and unreconciled. Each entry
  is raw material for distillation, not a page to copy.
- **memex temporal fields** are selection signals: skip entries with
  `SupersededBy` set or `ValidUntil` in the past; high `Importance` and
  recent `LastAccessedAt` argue for transfer.
- **weave memory / graph contrib** are already *team* rings (repo-scoped).
  Transfer from them into kb only when a lesson generalizes beyond the repo —
  otherwise it is already where it belongs and `--federate` finds it.

## Selection criteria, expanded

A memory earns transfer when all three hold:

1. **Durable** — will still be true and actionable in a month. Tells for
   failure: contains an in-flight state ("currently debugging"), a port
   number that changes per session, a TODO.
2. **Team-relevant** — changes what another agent would *do*. Tells for
   success: an operational rule ("never X on a live host"), a gotcha with a
   failure signature, orientation that took real effort to assemble, a
   decision with its why.
3. **Non-derivable** — not recoverable from the repo in reasonable time.
   Code structure, past fixes, and anything in CLAUDE.md/docs fail this
   (point at them instead of copying). The *interpretation* of repo facts —
   which of four test metrics is the release gate, which doc is stale — often
   passes even when the raw facts don't.

Priority order when time-boxed: operational rules that prevent unrecoverable
mistakes > gotchas with failure signatures > orientation/maps > decisions >
plain facts.

## Anti-patterns

- **The bulk dump.** `bashy kb import <private-memory-dir>` (once import
  exists) will *work* on frontmatter-md stores and is exactly wrong: raw
  private memory is transcript-shaped, self-referential, and unredacted.
  Import is for kb-shaped pages (a prior export, a curated bundle). Private
  stores route through this skill's distillation, always.
- **Transcript pages.** A page that narrates ("first I tried X, then Y
  happened...") instead of instructing ("X looks right but fails because Y;
  do Z"). Compress to the guardrail.
- **Self-validation.** Adding a page and immediately validating it yourself.
  The ladder means something because nomination and confirmation are
  different agents (or at minimum different sessions with independent use).
- **Copy-localization.** Pasting kb page bodies into your private memory.
  Copies rot silently; the kb page keeps getting corrected. Store pointers:
  "for <topic> consult `bashy kb show <slug>`".
- **Hostname leak.** Writing real hostnames/usernames/personal paths into
  pages that will travel (export, org catalog, skill promotion). Host-local
  pages MAY carry them when the fact is host-specific — tagged `host-local`
  — but the default is placeholders and role descriptions ("the always-on
  node", "the GPU host").
- **Re-nomination churn.** Running transfer twice and re-adding everything.
  The `xfer:<source>` tag is the marker; check it (`bashy kb list` /
  `bashy kb transfer`) before nominating from a source you've processed.

## Worked example — one agent trains the host

A real end-to-end run of the MENTOR loop (identifying details generalized):

1. **Setting.** An agent had accumulated ~50 private-memory notes across
   weeks of sessions in a multi-project monorepo: operational incidents,
   build constraints, release gates, naming decisions. Other agent tools on
   the same host (different vendors) shared none of it.
2. **Inventory + select.** Asked to transfer, the agent read its memory
   index, grouped the notes by subproject, and selected the durable,
   team-relevant, non-derivable subset — dropping session state, in-flight
   work, and anything the repos' own CLAUDE.md files already recorded.
3. **Distill.** Per subproject it wrote ONE orientation page ("what it is +
   the things that bite"), merging 6–10 memory files each: the subproject's
   role, then the operational rules ranked by cost-of-ignorance — each one a
   compressed incident ("upgrade no-ops on same-commit rebuilds — always
   `--force` when iterating; verify the swap took"). Descriptions were
   written as routing surfaces: "what + WHEN + trigger keywords".
4. **Reconcile.** Before each add it searched; the store's reconciler
   refused one near-duplicate (an earlier auto-generated page), which was
   superseded rather than forced past.
5. **Result.** Four `candidate` fact pages, cross-referencing each other,
   scoped `--repos` to their subprojects. A later session searching
   "deploy new <component> binary to a host" surfaced the right page
   first — the transfer paid off on the first downstream task.
6. **What was deliberately left out.** Secrets (vault-backed, never in
   pages), real hostnames where the rule generalized, and everything the
   repo already documented. The skip list went into the transfer report.
7. **The open half.** Validation: the pages stay `candidate` until another
   agent uses one in anger and runs `bashy kb validate <slug> --evidence`.
   The mentor's job ended at nomination — by design.

## Interplay with the rest of the toolchain

- **Delivery is automatic once you land the page.** `weave start` drops a
  `KB.md` of top matches into every workspace; foreman prepends a
  goal-matched block. Landing the page IS pushing it to future workers —
  finish the loop and the fleet gets it at spawn.
- **`bashy kb transfer` / `bashy kb sources`** (when available in your
  build): deterministic helpers that print detected source stores, related
  existing pages for a topic, already-transferred `xfer:` counts, and the
  decision menu — the ground-truth scaffolding for this skill. Absence of
  the verbs changes nothing about the process.
- **`bashy skills learn` / `promote`** is the sibling channel for
  procedure-shaped knowledge with a checkable contract; `promote` renders a
  human-review bundle and never commits on its own.
- **The kb trust ladder + doctor** (where available) close the loop:
  consultation records make "this page helped N times" visible, which is the
  evidence a validation wants.
