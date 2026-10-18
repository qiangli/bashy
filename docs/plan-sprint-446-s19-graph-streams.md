# Sprint 446 S19: Bashy coordination producers and address consumers

Story #2138 / 3e8962ee2915, Bashy continuation. Governing contracts are the
umbrella's `docs/sprint-446-master-execution-plan.md` Part 5 ("Pipes both
ways"), `docs/bashy-command-ecosystem-epic.md` (stream clarifications) and the
approved `bashy-cursor-v1` envelope. The framing itself is Yoke's
`pkg/lens/stream` at the version Bashy pins; Bashy adds no second encoder.

## Producers

The coordination verbs live in Yoke; Bashy owns their front door. At the
`setCobraArgs` chokepoint (`applyCommonOptions` → `installGraphStreams`,
`internal/agentos/graph_stream.go`) a verb whose canonical path has a declared
projection buffers its native `--json` document and re-emits it as one
`bashy-graph-v1` frame (header, sorted nodes, sorted edges, cursor trailer).
Classic text, a loom-v2 error envelope, a failed run, and any document the
projection does not recognise pass through byte-for-byte. Alias spellings
(`issue`, `agents`, `messages`) fold onto the canonical noun. Cert profile is
untouched (the chokepoint is skipped there).

| Path | Source | Nodes | Edges |
|---|---|---|---|
| `sprint board` (the sprint list) | `sprint:*` | `sprint:N` | `owned-by`, `links-run` |
| `sprint show N` | `sprint:N` | card, goals `sprint:N!/GOAL`, stories `todo:ID`, runs `run:REPO-N` | `contains` (card→goal→story, card→run), `next`, `owned-by`, `links-run`, `works-on` |
| `todo list` | `todo:FOLDER` | `todo:ID` | `assigned-to`, `story-of` |
| `todo show ID` | `todo:ID` | the item | `assigned-to`, `story-of`, `links` (in and out) |
| `agent list` / `agent show` | `agent:*` / `agent:NAME` | `agent:NAME` | `uses-tool`, `uses-model` |
| `claim list` | `claim:*` | `claim:KIND!/KEY` | `held-by` |
| `mb --json` / `mb show N` | `mb:*` / `mb:N` | `mb:N` | `from`, `to` |
| `meet show` | `meet:ID` (durable id) | meeting, agenda items | `contains`, `participant` (coverage counts) |

Projections keep declared fields only. Wall-clock-relative values (todo `age`
and `overdue`, sprint `resources` and `outcome_cost`, claim heartbeats, meet
coverage `last_status`) and the sprint thread are dropped, so one snapshot gives
one byte sequence and the header rev is a content identity. Edges may name an
entity the frame does not carry (an owner, an assignee); that address resolves
through its own command. The cursor provider is the noun; the query is the verb,
its operands and every set flag except `--json`.

Not projected, by command architecture: there is no `sprint list` verb
(`sprint board` is the list), no `claim show`, and `meet list` has no `--json`
(the front door refuses it with exit 2 as before).

## Consumers

A literal `-` reads addresses from stdin with `stream.ReadAddresses` — a frame
or a filtered record sequence (its node refs, in order) or plain addresses one
per line — and runs the verb once per address:

- `bashy sprint claim SPRINT -` takes `todo:ID`, `story:ID` or a bare id.
- `bashy mb send - BODY…` and `bashy mb send --to - BODY…` take `agent:NAME`,
  `person:NAME` or a bare name.

Every address is validated before the first run, so piping a whole
`sprint show` tree into `sprint claim` claims nothing and names the offending
address. The first failing run stops the batch with its own exit status.

## Evidence

`internal/agentos/graph_stream_test.go`: per-projection frame/order/cursor
binding/volatile-field checks, producer-order independence, the front-door
wrapper (alias, classic text, error envelope, failed run), the real Yoke `todo`
verb end to end over `--base-dir`, and the address consumers (stream, plain
locators, `--to -`, validate-before-act). `scripts/release-bar.py` accepts the
frame's declared header as the probe envelope (`scripts/test-release-bar.py`).
