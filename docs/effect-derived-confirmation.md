# Effect-derived confirmation — `@confirm`, `--what-if`, `--confirm` (B16, Sprint 216)

Story `c16217907cff` (S216.G, the one PowerShell item kept). The paired
harness contract is `rfcs/0001-agentic-yield.md` (B19, story
`528d134e6bd8`).

PowerShell's idea, borrowed without the shell: a function declares that it
supports confirmation (`SupportsShouldProcess`), and the runtime *generates*
`-WhatIf`/`-Confirm` for it. The function never implements a prompt, and the
prompt fires per operation (`ShouldProcess` call), not per script. Bash#
spells the declaration `@confirm()` and derives the operations from the one
effect source the surface already has.

## The one source of truth

The Command Atlas effects a dispatched command declares (`bashy commands
--view effects`; the vocabulary in `yoke/pkg/atlas`), resolved the way the
shell resolves the command: the atlas for curated verbs and in-process tools,
then the operator's ring for `bashy commands add` records (whose author
declared effects on the record). **No second effect list and no policy
engine.** The decision rung is the same ExecHandler seam that enforces the
`@guard` cap and, in Yoke's dag (B17), a target's `Effects:` — and it reads
the same atoms:

| declared effects | PowerShell analog | `--what-if` | default call | 
|---|---|---|---|
| only `pure` / `read` | no `ShouldProcess` call | runs | runs |
| `write`, `net`, `exec`, `remote`, `persist` | `ConfirmImpact` Medium | described, not run | runs (auto-confirmed, as under the default `$ConfirmPreference`) |
| `destroy`, `spend`, `cred`, `priv` | `ConfirmImpact` High | described with its token, not run | **confirmed** by the human, or the call yields |
| not classified at all | — | described as `unknown`, not run | **confirmed** (fail closed, as `Cap.Exceeded` reads an unclassified command) |

There is no `$ConfirmPreference` and no other PowerShell common parameter:
the gate set is a property of the vocabulary, not of a preference. That is
the deliberate deviation from PowerShell, where `-Confirm` also lowers the
threshold to Low/Medium.

## Behavior

`internal/agentos/confirm.go`: one native decorator (`@confirm`, registered
beside trace · guard · retry · require · ensure) plus one ExecHandler rung,
wired in `wireExec` just outside the dry-run handler — after autofix has
settled the argv it describes, outside the userland handler so in-process
tools are governed, inside audit so the ledger still records the outcome. It
is applied on the Bash# adapter waist (`sh/docs/bashpp-adapter-contract.md`):
the flag is bound at "bind and validate declared inputs", the decision is
made per invocation of a target, and an absent decorator is identity.

- `fn --what-if ARG…` — one stderr line per governed operation,
  `what-if: fn: would run ARGV (effects: E[,E…][; confirm TOKEN])`, none
  runs; pure/read commands run so the description is exact; the call exits 0.
- `fn ARG…` — each high-impact operation is put to the HUMAN through `bashy
  ask` over the terminal or an attended askpass helper (the channels the
  calling program does not own). `yes` runs it; anything else refuses it
  with status 126 (the shell's "found but not run", the status Yoke's dag
  uses for a cap denial) and the body continues, as PowerShell continues
  after "No".
- **No channel reaches a human → the call yields**: exit 6
  (`weavecli.ExitInputRequired`). That is the case when bashy orchestrated
  the run (`BASHY_AGENTIC`), a first-party harness owns input
  (`BASHY_ASK_HANDLER`), or there is neither a usable controlling terminal
  nor an attended askpass. The rendezvous rung is never waited on: an
  unattended call yields at once instead of blocking for a claim that may
  never come — the harness is the one that can reach the human. One line
  names the operation, its token, the cause and the exact resume form; every
  later side effect in the body is refused with `not run after yield` and
  status 6; the call's status is 6 whatever the body's last command said.
  Under `BASHY_AGENTIC=1` the line is a `weavecli` envelope
  (`error.code: "input_required"`).
- `fn --confirm=TOKEN:yes,TOKEN:no ARG…` — the resume form. A token is eight
  hex digits of SHA-256 over the operation's exact argv, so an answer binds
  to that operation and no other, in whatever order a replay dispatches
  them. An operation with no answer still asks (or yields). Bare `--confirm`
  is accepted and means the default. A malformed list is a decorator
  diagnostic, never a guess.

Only the FIRST argument is inspected, so a function's own flags are never
stolen. `@confirm` takes no arguments (there is nothing to configure: the
gated set is derived) and is source-only (the advice loader knows no such
decorator; the native refuses an advised call regardless).

## Bash OFF is identity

Decorator syntax does not parse in Bash or POSIX mode, the rung is not in
the `--posix` handler chain, `cmd/bash` never links `internal/agentos`, and
an undecorated function sees `--what-if` as an ordinary word.
`TestConfirmBashOffIsIdentity` pins all four.

## Evidence

- `internal/agentos/confirm_test.go` — source-derived cases with the
  provenance table in the file header: PowerShell/PowerShell `84a93015`
  (MIT) `CommonParameters.Tests.ps1` (`confirmimpact support: none / Medium /
  High under the non-interactive host`, `shouldprocess support -whatif`),
  `MshCommandRuntime.cs` (`DoShouldProcess`, `CanShouldProcessAutoConfirm`),
  `CommandBase.cs` (`enum ConfirmImpact`), `CommandBaseStrings.resx`
  (`What if: {0}`). Normal (none/medium/high-yes), boundary (empty, malformed
  and contradictory answers; flag in second position; unclassified command),
  failure (declined; unattended yield; envelope) and lifecycle (harness loop
  run → 6 → replay → 0; yield through `@ensure`) are all covered.
- `test/yield/` — the executable harness fixture and recorded transcript
  (B19): `run.sh [bashy]` drives a real `bashy --bashsharp` call that yields
  6, pauses, reads the recorded human answer, and replays with
  `--confirm=TOKEN:yes|no`, byte-diffing against `transcript.expected`.
- `bashy help confirm` — the operator summary.

## Out of scope (deliberately)

PowerShell's common-parameter surface, help parsing, completion, jobs and
registry breadth (narrowed out of the sprint); `$ConfirmPreference`;
prompting inside plain Bash scripts (that is dry-run territory: `bashy
--dry-run`); redirections (`> file` is not a dispatched command — the
dry-run open handler is the layer that sees it); advised `@confirm`; any
confirmation in `bin/bash` or `--posix`.
