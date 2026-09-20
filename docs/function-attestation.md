# Function-call attestation — Sprint 216 / B18 (story ed4495580ec2)

**What shipped.** When a decorated Bash# function (`@require` / `@ensure` /
`@guard` / `@trace` / `@retry`) or an `agentic function` completes under
`bashy`, ONE receipt is appended to the yoke skills/craft evidence ledger —
the same `skills.AttestRecord` a `bashy skill run` writes, into the same
`<store>/attest/<name>.jsonl`, read back by the same `bashy craft history`
(and `craft.ReadLedger`). No new store, no second record type, no model call.

## Why the ledger, not a new log

`yoke/pkg/craft/ledger.go` was written as a READER over evidence that already
existed, precisely to stop "the eighth parallel outcome log". A function
completing under a contract is evidence of exactly the kind a contracted skill
run is — a named thing, at a coordinate, whose checks held or did not — so it
lands where the read side already pools by name, capability and coordinate.

## The record

| field | function receipt |
|---|---|
| `name` | the function (a method is `Type.Method`) — also the ledger file |
| `attest.Skill` | `fn:<name>`, the identity that ran |
| `attest.Passed` / `attest.Failed` | clause verdicts as `<clause>:<check>` — `require:test -n "$1"`; a check after the first failure never runs and is never noted |
| `attest.Valid` | completed (status 0) and no check failed |
| `status` *(new, `*int`, omitempty)* | the call's exit status; `6` = `skills.AttestYield`, an agentic yield — a **handoff**, not a completion |
| `context_key` | this host's environment coordinate (`skills.HostCoordinate`, the same key `skills probe`/`craft fold` use) |
| `tier` | the executor, `bashy@<version>` (`local` on an unstamped dev build — the skills convention) |
| `store_revision` *(new, omitempty)* | `craft.Revision(store).String()` — `g1:f…k…a…/n` — taken BEFORE the append, so the receipt names the store it landed on |

Skill receipts leave `status`/`store_revision` absent, exactly as every
receipt written before them does; nothing on the read side infers one.

## How one call becomes one receipt

Every native decorator is wrapped by `attestSink.attesting`
(`internal/agentos/attest.go`). The OUTERMOST native rung on a call opens an
`attestFrame` on the ctx it hands down the chain; inner rungs on the SAME call
(keyed on the engine's `*interp.Call`, so a decorated callee inside the body
opens its own frame) find it and contribute verdicts
(`contracts.go` → `attestRecord`); when the outermost rung returns, the frame
is appended once. A clause under `@retry` re-records the same check per attempt
and the last verdict wins. A decorator that refuses its own arguments (an
`EDECO-NATIVE` failure) never ran the call and appends nothing.

An `agentic function` with no source decorator is reached through the
existing `interp.Advice` registration seam: `newAdviceCallback` adds the native
`attest` rung (a pure pass-through, rule id `bashy:attest`) to every agentic
registration. The seam fires only while Bash++ is active, which is what keeps
Bash OFF an identity. (The compiled host's `shellrt` slot has no advice seam,
so there an agentic function is attested only when a source decorator puts it
in a chain — recorded, not hidden.)

## Gating

- Never in `--posix`, never in `cmd/bash` (structural: the decorators and the
  advice callback are registered only on the agentic branch of `wireExec`).
- `BASHY_ATTEST=0|false|no|off` → off. An empty skills store → off.
- Store resolution follows `skills.DefaultStoreDir`: `$BASHY_SKILLS_DIR` →
  `$BASHY_HOME/skills` → `~/.config/bashy/skills`, read from the shell's env.
- A failed append is spoken once per shell on stderr
  (`bashy: attest: … — function receipts are not being recorded`) and never
  changes the call's own status: a ledger that silently stopped accruing would
  be an absence read as a clean record.

## Cases pinned by tests (`internal/agentos/attest_test.go`)

- **success** — `Valid`, status 0, every check under `Passed`.
- **contract failure** — precondition: status 3, `Failed=[require:…]`, no
  ensure; postcondition: status 3, `Passed=[require:…]`, `Failed=[ensure:…]`.
- **guard-denied body / plain failure** — status 1, require passed, ensure
  never judged (the postcondition judges a completed body only).
- **exit-6 / input required** — status 6, `Valid=false`, `Failed=[]`,
  `Yielded()`; `craft` counts it as `Yielded`, not `Failed`, renders `yield`.
- **plain undecorated function** — no receipt; **Bash OFF** — no ledger
  directory at all; **`BASHY_ATTEST=0`** — same.
- **one receipt per call** with trace+guard+require+ensure; a decorated callee
  inside a decorated body is its own receipt; `@retry` records the final
  attempt.
- **observable readback** — `craft.ReadLedger` (the `bashy craft history`
  path) reads every receipt back with status and store revision; the fixture
  `test/contracts/agentic-boundary.bpp` replayed on the built binary gives
  `summarize: RUNS 6 PASS 1 FAIL 4 yielded 1`.
- **unwritable store** — spoken once, the calls' outcomes unchanged.

## Dependency pin (the upstream seam)

The Yoke append API was unexported (`appendAttest`), and the record had no
field that could carry a status honestly, so the smallest upstream-compatible
seam was made in yoke — one commit, three files, all additive:

- `pkg/skills/run.go`: `AppendAttest` exported (refuses an empty store and a
  name that is not a ledger name); `AttestRecord.Status *int` and
  `AttestRecord.StoreRevision string`, both `omitempty`; `const AttestYield =
  6`; `AttestRecord.Yielded()`.
- `pkg/craft/ledger.go`: `Observation.Status/StoreRevision` projected;
  `Observation.Yielded()`; `Stats.Yielded` (in `Runs`, in neither `Passed` nor
  `Failed`).
- `pkg/craft/cmd.go`: `history` row carries `yielded`, `--all` renders `yield`.

Pinned in `.sibling-pins` as **`yoke=efcd8817d4cde14b74258798b14cff530f84c84c`**
(branch `agent/bashy-issue-41-attest-seam`, parent `c1ccbc9`, trailers
`Sprint: #216 / Story: #545 / Story-ID: ed4495580ec2`). CI clones the pin from
GitHub, so that commit must be pushed to yoke's origin before this lands —
the worker does not push. bashy's `go.mod` also now lists
`github.com/dhnt/dhnt` as a direct requirement (`attest.go` builds a
`dhntskills.Attestation`); the version is unchanged.
