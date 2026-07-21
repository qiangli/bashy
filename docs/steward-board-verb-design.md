# `bashy board` — the steward + conductor fleet console

**Status:** design of record (2026-07-20). Generalizes the ad-hoc session
dashboard (a python script → Artifact) into a first-class bashy verb. Local-first,
pure-Go, no network. Read-only observability (report, not act).

## The insight: bashy already has the three work layers

Steward work is not just weave runs. bashy already models the whole surface:

| Layer | Verb | Holds |
|---|---|---|
| **Backlog / chores** | `bashy todo` | todo→doing→done items, auto-scoped (repo `docs/todo/` committed, or personal `~/.bashy/todo/`). Home for NON-fleet work: releases, pins, CI, doc/memory chores. |
| **Durable goals** | `bashy sprint` | the cross-repo kanban ABOVE weave — one initiative spanning repos, with continuity + conductor lease + run links. `bashy sprint` already renders a user-global board (`sprint board --json` → `stories`). |
| **Execution** | `bashy weave` | per-repo agent runs. `weave list --all` already aggregates every queue on the machine. |

The board is the **view that unions all three**, with the natural hierarchy
`sprint → its weave runs`, plus standalone todos and standalone runs.

## Verb surface: role namespaces (`skill` + `dashboard`)

`bashy steward` and `bashy conductor` are today **skills** printed as bare verbs.
Generalize each into a role namespace with two subcommands — the how-to and the
live view — so the view sits right next to the role that uses it:

```
bashy steward   skill        # the steward skill text (what bare `bashy steward` shows today)
bashy steward   dashboard    # the steward console — machine-global union (this doc)
bashy conductor skill        # the conductor skill text
bashy conductor dashboard    # the conductor console — one sprint (P2)
```

- **Back-compat:** bare `bashy steward` / `bashy conductor` keep printing the
  skill (== the `skill` subcommand), so nothing that calls them today breaks.
- Both dashboards are thin, role-scoped entry points into ONE shared engine
  `pkg/board`; they differ only in scope (steward = global union; conductor =
  one sprint). `bashy board` MAY also exist as the un-namespaced engine verb, but
  the role verbs are the intended front doors.

## Two views, one renderer

The same board serves two ROLES; they differ only in SCOPE and default sources:

### Steward view — the god view (machine-global)
`bashy steward dashboard` (`--all` is implied; global): union of **all** todos +
**all** sprints + **all** weave runs across every repo on the machine. Answers
"what is the whole fleet doing, everywhere, and what needs me?" Lanes by state;
a **Needs-steward** lane surfaces ready-to-merge runs, blocked todos, and sprints
awaiting a converge decision. Agent-load-by-band across the whole machine.

**The steward view is a stack of collapsible panels** (the `--html` view uses
`<details>`; the terminal view uses a `--expand <panel>` flag, collapsed by
default so the default output stays glanceable). Each panel is a summary line
that expands to its full contents:

- **Agents** — collapsed: the in-flight load-by-band bars. Expanded: the **whole
  fleet roster** (all agents, not just in-flight), each with band · model ·
  reliability · **availability** (installed? on PATH? cooling down? signed in?
  model usable? — sourced from `bashy weave fleet`), and idle/working/cooling
  state. So the steward sees not just who is busy but who is *available to take
  work*.
- **Todo** — collapsed: counts by state. Expanded: the **full todo list** (all
  scopes — every repo's `docs/todo/` + the personal list), grouped by
  repo/scope, each item with state and age.
- **Sprints** — the durable initiatives, each expandable to its runs + gate.
- **Runs** — the weave execution lanes (the current kanban).
- **Future stats panels (extensible)** — the panel list is open-ended: CI health
  (`gh run list`), release/version drift, sibling-pin staleness, per-agent
  cost/throughput, campaign burn-down. Design the panel set as a registry so a
  new stat is a new `Panel` implementation, not a rewrite. Ship P1 with Agents +
  Todo + Sprints + Runs; leave the registry seam for the rest.

### Conductor view — one initiative
`bashy conductor dashboard [--sprint <id>]` (or run inside a repo → the active
sprint): scoped to
ONE sprint the conductor owns — its stories/runs (per-repo weave), the **gate /
acceptance** status, the **continuity** record (resume brief), the **lease**
holder, and the agents seated on it with their bands. Answers "is MY initiative
converging, who's stuck, what's left before the gate is green?" This is the
productized form of the conductor skill's loop view; `bashy sprint` is its seed.

Both render identically:
- **default** → terminal kanban (lanes by state; each run row `#id · label · tool
  · L<band> · model · elapsed/eta`; summary: counts, ETA median, agent-load-by-band).
- `--json` → versioned envelope `bashy-board-v1` (rows + rollups + the scope/role).
- `--html [--out]` → a **self-contained**, theme-aware, responsive page (the Go
  port of the session `gen-board.py`; inline all CSS, NO external hosts — CSP-safe).
  This is what a steward/conductor publishes as a shareable artifact.

## Data model (shared)

```
Board{ Role(steward|conductor), Scope, Sprints[], Rows[], Rollup{ByState, ByAgentBand, Merged, EtaMedianSecs} }
Sprint{ ID, Title, Column, GateState, LeaseHolder, ContinuityRef, RunRefs[] }
Row{ Source(todo|sprint|run), Repo, ID, State, Tool, Band, Model, Label, Points, ElapsedSecs, DurSecs, SprintID? }
```

- **Sources** are pluggable: a `todo` source, a `sprint` source, a `weave` source,
  each yielding `Row`s. Steward = all sources, all repos; conductor = one sprint's
  runs + its backlog.
- **Band/model** resolution lives in `pkg/fleet`:
  `fleet.ResolveLaunchModel(tool, launchModelDisplay) (band int, canonical string)`
  — the launch record stores the provider DISPLAY string
  (`"Gemini 3.1 Pro (High)"`); resolve to canonical by normalized-substring match
  against the catalog's canonical names, LONGEST match first, band read live from
  the catalog (a re-peg shows up with no code change). Band 0 / raw string when
  unresolvable — never guess.
- **ETA** = median duration of completed same-kind runs; omit when n is too low.

## Prerequisite gap to close in this work

- **`bashy todo --json`** does not exist yet — the board needs it. Add a versioned
  `--json` envelope to `bashy todo` (list) first; it is also generally useful.
- `bashy sprint board --json` already exists (reuse its `stories`).
- `weave list --all --json` already exists (reuse its items).

## Scope discipline

- **Generalized, NOT campaign-specific** — no hardcoded "Sprint 12 / POSIX". The
  board describes whatever is in the three layers; any title comes from the sprint
  or a neutral default.
- **Read-only.** Never merges/starts/kills — those stay explicit verbs (report/
  author split).
- **No silent truncation** — if a lane is display-capped, say how many were dropped.

## Files

- `coreutils/pkg/board/` *(new)* — the `Board` model, the three `Source`s, and the
  three renderers (text, json, html). Keep the HTML template here (or a
  `board/boardhtml/` subpkg).
- `coreutils/pkg/todo/` — add the `--json` list envelope.
- `coreutils/pkg/fleet/` — `ResolveLaunchModel` + a table test.
- `coreutils/pkg/weave/`, `coreutils/pkg/sprint/` — expose the `--all` / `board`
  data to the board sources (reuse existing list walkers; no duplication).
- `coreutils/pkg/atlas/` + `bashy/internal/agentos/atlas.go` — atlas entries for
  `bashy steward dashboard` (+ `steward skill`) and, in P2, `bashy conductor
  dashboard` (coverage tests + e2e dispatch gate fail without them). Wire the
  `steward`/`conductor` skills as the `skill` subcommands.

## Gate (RUN, do not self-report)

`go build ./...` + `CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build ./...` clean;
new tests green (renderer golden text+json; band-resolver table incl. the
ambiguity cases `gpt-5.5` vs `gpt-5.6-sol`; `todo --json` envelope; an `--html`
self-contained smoke asserting no external hosts); `bashy commands` lists
`bashy board`; the e2e dispatch gate passes. Match repo style; HTML CSP-safe.

## Phasing (so it lands incrementally)

- **P1** — `pkg/board` model + `fleet.ResolveLaunchModel` + `todo --json` +
  **`bashy steward dashboard`** (all sources, global) with text + `--json` +
  `--html`, and `bashy steward skill` wired to the existing skill. Retires the
  session python.
- **P2** — **`bashy conductor dashboard [--sprint <id>]`**: gate/acceptance +
  continuity + lease + seated-agent bands, composing `bashy sprint`; plus
  `bashy conductor skill`.
