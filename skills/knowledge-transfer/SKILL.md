---
name: knowledge-transfer
description: >-
  Transfer knowledge from one agent to others through the host knowledge base
  (`bashy kb`) — the way a senior developer trains the team: distill private
  experience into shared, validated pages, and onboard from them before a
  task. Two loops: MENTOR (turn your private memory / in-context recall into
  reconciled kb pages other agents inherit) and MENTEE (consume the kb before
  a task, then validate-through-use so candidate knowledge becomes team
  knowledge). Use when an agent holds durable knowledge others on this host
  will need (operational rules, gotchas, orientation, runbooks), or when
  onboarding onto unfamiliar ground. NOT for session-scoped scratch, facts
  derivable from the repo (code, git history, CLAUDE.md), or bulk-copying a
  private memory store verbatim — kb pages are distilled strategy, never
  transcripts.
---

# knowledge-transfer — one agent trains the others, via `bashy kb`

You are moving knowledge between minds. The medium is the host knowledge base
(`bashy kb` — the team wiki every agent on this host shares); the discipline
is an agile team's: the mentor distills, the wiki carries, and knowledge only
becomes *team* knowledge when a second developer uses it successfully.

**Three rules govern everything:**

1. **Transferred ≠ validated.** Every page you transfer lands as `candidate`.
   Only a *different* agent (or a later session), after actually using the
   page, promotes it with `bashy kb validate <slug> --evidence "..."`. Never
   validate your own fresh transfer — nomination and confirmation are separate
   acts.
2. **kb reads foreign stores; kb never writes them.** You may read any memory
   store you own or are pointed at; the only write target is the kb. Never
   write into another agent's private store — leave pointers, not copies
   (see MENTEE step 4).
3. **Distill strategy, not transcript.** One page = one reusable claim-cluster
   with a "what + WHEN this applies" description. If you are pasting session
   narrative, stop and compress.

## MENTOR loop — share what you know

### 1. Inventory the sources

Knowledge comes from two places:

- **(a) Your private memory store** — file-based notes your tool keeps.
  Common stores on a host (probe what exists; absence is normal):
  - Claude Code: `~/.claude/projects/<project-id>/memory/*.md`
    (frontmatter markdown + a `MEMORY.md` index)
  - ycode memex: `~/.agents/ycode/memory/*.md` and per-project
    `<repo>/.agents/ycode/memory/*.md` (frontmatter markdown)
  - weave campaign memory: `~/.bashy/weave/<tag>/memory.jsonl`
  - repo graph contributions: `<repo>/.agents/bashy/graph/contrib.jsonl`
- **(b) Your in-context recall** — what you know right now from the session
  or your training that others will need and cannot derive.

`bashy kb sources` (when available) prints the detected stores with entry
counts; otherwise probe the paths above directly.

### 2. Select — three filters, all must pass

- **Durable**: still true and useful next month. Session state, in-flight
  debugging, and one-off values fail this.
- **Team-relevant**: another agent (possibly a different tool) doing work on
  this host would act differently for knowing it.
- **Non-derivable**: NOT already recorded in the repo (code, tests, git
  history, CLAUDE.md, docs/). The kb complements the repo; it never mirrors it.

Source-specific hygiene: skip private-memory entries marked superseded
(memex `SupersededBy` set) or expired (`ValidUntil` in the past); prefer the
newest version of a corrected note; convert relative dates to absolute.

### 3. Redaction gate — before anything is written

- **Secrets/tokens/keys: never.** Not even host-locally.
- **Real hostnames, usernames, personal paths**: acceptable in host-local
  pages ONLY when the fact is inherently host-specific — and then tag the
  page `host-local` so a future export or promotion can warn. When the
  knowledge generalizes, write it with placeholders and role descriptions
  instead.
- Knowledge about proprietary code destined beyond this host follows the
  license boundary: describe public wire behavior, not private internals.

### 4. Route — the procedures-vs-facts fork

- **Executable procedure with a checkable contract** (a gate that can attest
  it ran correctly) → make it a *skill*: `bashy skills learn <dir>` (verified
  admission), later `bashy skills promote` for upstream review.
- **Prose know-how, judgment steps, operational rules, orientation, gotchas**
  → a kb page: `bashy kb add --type runbook|gotcha|lesson|decision|fact`.
- When both exist, cross-link: the kb page names the skill; the skill's
  reference names the kb slugs.

### 5. Distill and reconcile — one page at a time

For each selected claim-cluster:

```
bashy kb search <topic terms>        # ALWAYS first — does a page already exist?
```

- Exists and right → `bashy kb validate <slug> --evidence "..."` or extend via
  `bashy kb update <slug> ...`
- Exists and wrong → `bashy kb supersede <slug> ...` (correction stays linked)
- Missing → `bashy kb add` with:
  - `--description` = *what + WHEN this applies*, with trigger keywords — it
    is the routing surface other agents search against;
  - `--type` fact/gotcha/lesson/runbook/decision (gotchas and failure
    guardrails are the highest-value pages — "X looks right but Y");
  - `--evidence` = how you know (commit, incident date, command output);
  - `--repos`/`--os` scope when the page only applies there;
  - **`--tags xfer:<source>`** (e.g. `xfer:claude-memory`, `xfer:memex`,
    `xfer:recall`) — the idempotence marker: a later transfer pass sees what
    this source already contributed and doesn't re-nominate it;
  - plus `host-local` when the redaction gate said so.

`kb add` reconciles automatically and refuses near-duplicates — when it
points at an existing page, that is the process working: update or supersede
that page instead of forcing.

### 6. Link the cluster

Related pages you created or touched should reference each other in their
bodies (`[[slug]]` style). A page an agent finds should lead to its siblings.

### 7. Report

End with a transfer summary: pages added / updated / superseded / validated /
skipped-with-reason (failed a filter, duplicate, redacted). The summary is
what the requesting human reviews — make the skips visible, they carry the
judgment.

## MENTEE loop — learn what the team knows

1. **Search before the task**: `bashy kb search <task terms>` (weave/foreman
   workers get top matches injected at spawn as `KB.md` — read it). Open the
   full page for anything that matches: `bashy kb show <slug>`.
2. **Trust by status**: `validated` pages are team-confirmed; `candidate`
   pages are one agent's nomination — use them, but verify claims against
   current code before asserting them onward.
3. **Validate through use**: when a page materially helped and proved correct,
   promote it — `bashy kb validate <slug> --evidence "used in <task>, held"`.
   When it proved wrong, fix it: `update` (small drift) or `supersede`
   (wrong). This is the moment team knowledge is actually born.
4. **Localize with pointers, not copies**: record in your OWN private memory
   "for <topic>, consult `bashy kb show <slug>`" — never copy page bodies into
   your store (copies rot; the kb page stays the single maintained truth).
5. **Retro**: after the task, `bashy kb retro <task terms>` and decide ONE of
   ADD / UPDATE / SUPERSEDE / VALIDATE / NOOP.

## Worked example and deep guidance

`reference.md` (bundled) carries the source-store reference table, the
selection criteria expanded, the anti-pattern list, and a worked end-to-end
example: a real session in which one agent distilled its private memory into
four subproject orientation pages that other agents then consumed.
