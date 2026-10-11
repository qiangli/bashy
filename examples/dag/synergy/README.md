# `synergy` — the S12 bounded composition fixture

Sprint #418 · Story #1893 (`67380890250f`, "S12 integrated synergy DAG
fixture and bounded fake gate"), child of umbrella S12 `07917b37185b`.

`dag.md` here drives the whole S12 sequence through the current Bashy CLI,
and `internal/agentos/synergy_dag_test.go` runs it against the real binary
with the two agent turns stubbed. This README is the implementation plan of
record for the story, the evidence for what the bounded run does and does
not prove, and the list of live gates that stay with the manager.

## The sequence

| # | target | real Bashy surface | ID the ledger carries | where it is parsed from |
|---|---|---|---|---|
| 1 | `init` | `bashy --version` | — | writes `lib.sh` and resets the ledger |
| 2 | `unpaired-refusal` | `meet dm AGENT --channel C --sender S TEXT` | single-use pair code | the refusal line `pair code <CODE> … message not delivered` |
| 3 | `pair-approve` | `app pair approve CODE --allow ask,approve`, `app devices --json` | device id | `paired … as <ID>`; cross-checked in the devices JSON |
| 4 | `paired-input` | `meet dm …` (now relayed to the agent), `meet list`, `meet contributions --json` | DM room id | the `ID` column of the `dm-*-<channel>-<sender>` row |
| 5 | `claude-plan` | `kb note add --candidate`, `kb list --json`, `todo add --json` | kb slug, kb page id, todo id | `added note <slug>`; `.pages[slug].id`; `.id` |
| 6 | `codex-implementation` | `todo start`, `chat --agent AGENT --json -m …` | todo id prefix; **none** for chat | `doing <prefix> — …`; the `bashy-chat-v1` envelope has no turn id |
| 7 | `approval-request` | `approve request --json -- bashy todo done ID` | approval id (+ token) | `.id`, `.token`, `.state` must be `pending` |
| 8 | `approve` | `meet dm … "/approve ID"`, `approve show ID --json` | approval id | `allow <id> (…)`; `.state=allow`, `.decision.by=<channel>:<sender>` |
| 9 | `done-notification` | `todo done`, `todo show --json`, `notify --json`, `chat timeline`, `activity tail` | todo id prefix; **none** for notify / timeline / activity | `done <prefix> — …`; the others carry no id (see below) |

Every command's stdout, stderr and exit status are kept under
`$STATE_DIR/raw/<step>.<command>.{out,err,status}`. The ledger
`$STATE_DIR/event-ids.ndjson` has one line per receipt:

```json
{"step":"approval-request","command":"approve request","exit":0,
 "id_source":"approve request --json: .id (token .token=64339077)",
 "event_id":"13186caae7c3385ceec011e611aa5a1b",
 "receipt":"raw/approval.approve-request.out"}
{"step":"done-notification","command":"notify","exit":0,
 "id_source":"bashy-bus-v1 receipt .state=unverified","event_id":null,
 "absence":"bashy-bus-v1 carries schema_version, state, principal, to and subject but no id",
 "receipt":"raw/done.notify.out"}
```

`event_id` is only ever a value parsed out of the named receipt. When the
current CLI prints no ID for a step, the line says `"event_id": null` and
quotes exactly what the receipt did carry. Nothing invents an ID.

## What is bounded, and how

- **Agent turns.** Bashy launches the planner (`claude-fable5.1`, tool
  `claude`) through `meet dm` and the implementer (`codex-gpt5.6-terra`,
  tool `codex`) through `chat`, exactly as a live run would. The test points
  those launches at deterministic `sh` scripts through the documented
  operator override `BASHY_TOOL_BINARY_CLAUDE` / `BASHY_TOOL_BINARY_CODEX`
  (`fleet.ToolBinaryOverrideEnv`). The DAG never names a stub; unset the two
  variables and the same file runs the pinned CLIs. The stubs log their argv,
  which is how the test proves the implementer was addressed with the todo
  and kb page this run created, and that the unpaired refusal and the
  `/approve` command launched no agent at all.
- **Messenger ingress.** `bashy meet dm --channel C --sender S TEXT` *is* the
  gateway front door (a relay feeds argv or stdin). The DAG plays the relay.
- **Notification author.** `bashy notify` refuses an anonymous sender, so
  the done step claims the implementer's seat (`BASHY_AGENT` /
  `BASHY_PRINCIPAL`, override with `NOTIFY_AS=…`) unless the invoking
  session already carries an identity, as a live agent session does.
- **Host hygiene.** The todo goes to the personal list (`TODO_SCOPE=--user`)
  and the kb page to the host ring (`KB_RING=host`); running inside a
  repository never edits its committed `docs/todo/` or `docs/kb/`. The test
  additionally runs under a throwaway `HOME`/`BASHY_HOME` with none of the
  invoking agent's `BASHY_*` identity.

## Fail-closed by construction

Bodies in `bashy dag` do not stop at a failing line on their own (a `false`
followed by `echo` reports the target as ok). Every body here therefore
opens with `set -euo pipefail`, routes each command through `run_step`
(record stdout/stderr/status first, *then* return the status), and checks
the receipt before trusting exit 0:

- a sender that is **not** refused at step 2 fails the run (that is what a
  second pass against the same host does — the test asserts it);
- a missing pair code, device id, room id, slug, page id, todo id, approval
  id or token fails the run (`need`);
- an approval that is not `pending`, a `/approve` that does not answer
  `allow <the requested id>`, or a decision not recorded as `by
  <channel>:<sender>` fails the run;
- `todo start`/`todo done` acting on anything but the created todo fails
  the run — no todo is ever selected from a listing.

## Exact absences in the current CLI (recorded, not invented)

Measured on the binary built from this checkout (darwin/arm64, 2026-10-11):

- `bashy chat --json` (`bashy-chat-v1`) carries `agent`, `nick`, `model`,
  `cwd`, `args`, `exit_code`, `output` — no turn or session id.
- `bashy notify --json` (`bashy-bus-v1`) carries `schema_version`, `state`,
  `principal`, `to`, `subject` — no id. To a paired messenger identity the
  state is `unverified`.
- `bashy chat timeline` renders the host room timeline (`inbound`,
  `approval #pending`/`#allow`, `todo #doing`/`#done`, `notify` lines) with
  no id column and no `--json`. `bashy events`, which the bus help names as
  the reader of that log, is not mounted in this binary (`command not found`).
- `bashy activity tail` prints no records: the journal lists its sources but
  no producer writes to it in this binary.
- `meet dm` prints the relayed reply only; the room id is read from `meet
  list`. The room transcript (`meet contributions --json`) has no per-event
  ids either.

If a later binary starts emitting an id in the chat or notify receipt, the
test fails on purpose (`receipt now carries an "id" field`) so the ledger is
updated to parse it.

## Running it

```sh
make build-bashy                                   # bin/bashy from this checkout
GOWORK=off go test -short -run TestSynergyDAG ./internal/agentos/   # the bounded gate (≈2 s)
bin/bashy dag -f examples/dag/synergy/dag.md --check                # syntax / graph
bin/bashy dag -f examples/dag/synergy/dag.md -n done-notification   # ordered plan
```

By hand, with stubbed turns, against a scratch home:

```sh
H=$(mktemp -d); printf '#!/bin/sh\necho PLAN\n' >"$H/claude"; printf '#!/bin/sh\necho DONE\n' >"$H/codex"; chmod +x "$H"/claude "$H"/codex
env -i PATH="$PATH" HOME="$H" BASHY_HOME="$H/bashy-home" BASHY_HINTS=off \
    BASHY_TOOL_BINARY_CLAUDE="$H/claude" BASHY_TOOL_BINARY_CODEX="$H/codex" \
    bin/bashy dag -f examples/dag/synergy/dag.md -B STATE_DIR="$H/state" done-notification
cat "$H/state/event-ids.ndjson"
```

Live (no stubs): leave the two `BASHY_TOOL_BINARY_*` variables unset, run from
a session that carries an agent identity, and revoke a previous pairing first
(`bashy meet dm --channel fixture --sender alice --revoke`) — the fixture
refuses an already-paired sender rather than passing through.

## What this does NOT prove — manager-owned live gates

Not claimed by this story, not green here, to be run and recorded by the
manager separately:

1. the Linux full-DAG run with the pinned live `claude` and `codex` CLIs
   (no `BASHY_TOOL_BINARY_*`);
2. real app pairing from a phone/messenger relay rather than argv ingress;
3. the two-host approval (request on one host, `/approve` from a paired
   sender on another);
4. `make test-bash` / the GNU Bash 5.3 gate and the 3-OS CI matrix — this
   change touches no shell semantics, but the push gates are theirs to run.

Verified in this run: `bin/bashy dag --check` (9 targets, no cycles),
`GOWORK=off go test -short -run TestSynergyDAG ./internal/agentos/` (both
tests pass against `bin/bashy`), and the broader
`GOWORK=off go test -short ./internal/agentos/...` as recorded in the
delivery handoff.

## Lineage

- Weave #33 (AGY) exited -1; its skeleton survives as
  `79ef3f1970043e60a3d3a2ad830d995b033d4832` (`refs/salvage/abandoned-33`).
  Not accepted.
- Weave #34 (Codex Terra) submitted `2c6b1d88`: hard-coded ledger labels
  (`pair-request`, `approval-pending`), `|| true` on the approval request,
  an `echo`-grade implementation turn, a shell stub standing in for the
  whole `bashy` binary, and an unregistered agent name. Not accepted;
  survives at `refs/salvage/abandoned-34`.
- Weave #35 (GLM/Ycode) produced no source.
- This rework (#36) keeps #34's step names and state layout, replaces every
  ledger value with one parsed from a receipt, drives the real binary, and
  adds the fail-closed second pass.
