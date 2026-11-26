# Field-loop end-to-end gate

Sandbox e2e with fakes for the Sprint 730 field loop (F4). It exercises the
existing shaping workflow from three sources against the real `form`
shapeware app, in a throwaway HOME, with no network, no real GitHub and no
browser. It finishes in a few minutes; waits are configurable.

## Fakes and fixtures

- `fixtures/tools/codex` — fake shaping agent. It answers the delegate's
  login-status probe and makes one deterministic fix in the candidate
  checkout: it removes the injected `FIELD_LOOP_5XX_MARKER` line from
  `form.bsh` when present, otherwise appends the field-loop note.
  Bound with `bashy tools set codex --set cli.binary=...` and pinned as
  `codex:gpt-field-loop`; a delegate dry run proves the binding.
- `fixtures/issues.json` — issue file for `shape-feed --source file:...`:
  one owner issue (`field-owner`, #101) and one non-owner issue
  (`someone-else`, #102) that must be skipped.
- `fixtures/tools/open` and `fixtures/tools/xdg-open` — no-ops on `PATH`, and
  `BROWSER=/usr/bin/true`, so nothing ever opens a browser.

## Run on a claimed gate lane

Claim the gate lane with `bashy claim` before running. Build a Linux
candidate first, then run the dag target (syntax-check only on the dev box;
the conductor runs this on the gate lane):

```sh
cd bashy
GOWORK=off CGO_ENABLED=0 go build -o bin/bashy ./cmd/bashy
export FIELD_LOOP_GATE=1
export FIELD_LOOP_EVIDENCE=/path/to/fresh-evidence/field-loop-run1.log
bashy dag test-field-loop-e2e
```

Environment:

| Variable | Meaning | Default |
|---|---|---|
| `FIELD_LOOP_GATE` | must be `1` (fail-closed otherwise) | — |
| `FIELD_LOOP_BASHY_BIN` | bashy binary under test | `bin/bashy` |
| `FIELD_LOOP_EVIDENCE` | fresh evidence log path (must not exist) | — (required by dag target) |
| `FIELD_LOOP_WAIT_SECONDS` | per-step wait budget in seconds | `120` |
| `FIELD_LOOP_OWNER` | owner login for the feed filter and todos | `field-owner` |

## Scenarios

- **(a) Incident heal.** The harness registers `form` from a throwaway git
  copy, injects the 5xx marker, reports the incident, and waits for the
  console incident repair (fake agent) to activate a healed revision.
  `bashy shape auto log` must show `trigger`, `attempt`, `verdict` and
  `activation`, and `bashy shape restore` must bring the previous revision
  back.
- **(b) Owner feed.** `shape-feed form --source file:fixtures/issues.json
  --apply` files exactly one todo (`origin=feed`, `reporter=field-owner`),
  enters `shape wish --source feed:ID` and, once verified, `shape approve`
  activates it (F3b interface). The non-owner issue files nothing.
- **(c) Schedule + off switch.** The same feed is added with
  `bashy schedule add` and run once with `bashy schedule run`. After
  `bashy shape auto off`, a second run files nothing and
  `bashy shape auto log` shows the `refusal`.

A failure leaves the todo from (b) in `known` status with what was tried
(shape-feed's failure path) and the evidence log records every command.

Each run writes a fresh evidence log with the sha256 of the runner,
fixtures and `dag.md`, the binary hash, every command and its output.
Keep the passing log on the gate lane (evidence recorded once on the
candidate).
