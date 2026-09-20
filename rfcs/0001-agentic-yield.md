# RFC 0001 — Agentic yield: exit 6 is a handoff, not a failure

Status: shipped (Sprint 216, B19, story `528d134e6bd8`). Producer: `@confirm`
(B16, `docs/effect-derived-confirmation.md`). Fixture: `test/yield/run.sh`.

## Problem

A command run BY an agent does not own its stdin or stdout — the harness does
(`docs/plan-bashy-ask-human-input.md` measured it: Claude Code `setsid`s its
children, so even `/dev/tty` is ENXIO). When such a command reaches an
operation only a human may approve, every honest option looks like this RFC
and every dishonest one is worse: block forever on a terminal that isn't
there, guess the answer, or fail with an exit code the harness reads as "the
command is broken, try something else".

Each major harness already has the loop this needs — pause, ask the human,
resume — but only at ITS OWN tool boundary, for approving the command before
it starts (see §Provenance). None of them defines a way for a command that is
already RUNNING to raise its hand. This RFC is that missing half, and it is
deliberately the smallest thing that works: one exit code the front door
already uses, one stderr line, one resume flag that already exists.

## The contract

1. **Yield.** A bashy-run call that needs an answer nobody present can give
   exits **6** (`weavecli.ExitInputRequired` — the same code and
   `input_required` envelope code every front-door verb already uses; no
   second protocol). Before exiting it prints ONE line naming what it needs:

   ```
   clean: input required: confirm e9d85167: rm -rf victim (effects: destroy): <cause>; resume with clean --confirm=e9d85167:yes ... or --confirm=e9d85167:no ...
   ```

   Under `BASHY_AGENTIC=1` the same line is a `weavecli` JSON envelope
   (`status: "error"`, `error.code: "input_required"`, the resume form in
   `error.message`). The format is a contract, never sniffed.

2. **Nothing runs past the yield point.** The operation that needed the
   answer ran **zero times**, and every later side effect in the body is
   refused (`not run after yield`, status 6). Pure/read commands may still
   run; the call's status is 6 whatever the body's last command said.

3. **The harness pauses and asks a human.** With whatever UI it already has
   for its own approvals. It MUST NOT answer from a model, a default, or a
   timeout — see §Never fabricate.

4. **The harness replays** the same call with the answer bound to the token:
   `clean --confirm=e9d85167:yes …` (or `:no`). It does not resume a paused
   process; it runs the call again. §Bounded replay says why that converges.

A yield is a HANDOFF: exit 6 with the yield line means "a human is needed
here", never "this failed" — an attestation records it as `yielded`, not
`FAIL` (`docs/function-attestation.md`).

## Resume

The resume form is `--confirm=TOKEN:yes|no[,TOKEN:yes|no…]`, in the call's
first argument position (B16's flag — this RFC adds no new one).

- A **token** is eight hex digits of SHA-256 over the operation's exact argv,
  NUL-joined. It is argv identity: stable across replays and machines,
  distinct across operations, position-independent — so an answer binds to
  that operation and no other, in whatever order a replay dispatches them.
- The answer binds to the token, not to the run: extra answers for
  operations the replay never dispatches are simply never consulted.
- An **unanswered** high-impact operation in a replay asks again — and,
  unattended, yields again, naming its own token. Absence of an answer is
  never "no".
- A **malformed** answer list (`--confirm=`, a short token, `TOKEN` answered
  both ways) is a diagnostic that refuses the whole call before the body
  runs. A resume is spelled exactly or not at all.
- `:no` is an explicit refusal: that operation reports 126 (found, not run)
  and the body **continues**, exactly as an attended human "no" behaves.

## Harness

The loop, as `test/yield/run.sh` performs it against a real binary:

```
run the call
└─ exit 6?  → take TOKEN and the resume form off the one yield line
             (or the envelope's error.message)
             ask the HUMAN yes/no, with your own approval UI
             replay:  same call, --confirm=TOKEN:<answer> prepended
             └─ exit 6 with a NEW token? → ask again (next operation)
             └─ exit 6 with the SAME token? → stop; you did not answer it
             └─ anything else → done: the call's ordinary result
```

`--what-if` is the harness's free preview: it lists every governed operation
and each high-impact token before anything runs, so a harness can collect all
answers in one question if it prefers.

## Bounded replay and idempotency

- **One yield, one operation.** A yield names the first unanswered
  high-impact operation and stops the body's side effects there.
- **Progress bound.** Each replay round either completes or yields a token
  not previously named. A body with G high-impact operations converges in at
  most G replays. The same token appearing twice means no progress — the
  answer was not supplied, or the body's argv is nondeterministic — and the
  harness must stop and re-ask rather than loop (fixture case 5; the
  space-time advisor's doomed-loop rule, applied to yields).
- **At-least-once, not exactly-once.** A replay runs the body from the top:
  the pure/read prefix and the auto-confirmed medium-impact operations run
  again. The guarantee sits on the ASKED operation — it ran zero times
  before its `yes` and runs once after it. A body written for handoff keeps
  its side effects replayable (the fixture's `rm -rf` and `mkdir` are), the
  same discipline any at-least-once step already owes.

## Never fabricate

The answer must originate with a human: typed at a terminal, given through
the harness's approval UI, or recorded by the human beforehand as standing
authorization. A harness MUST NOT synthesize `yes` or `no` — not from a
model, not from a default, not from a timeout. That rule is why the yield
exists at all: bashy would rather stop the world with exit 6 than let the
question fall to whoever happens to be listening. The fixture's driver obeys
its own rule — its answer is a recorded human reply (`test/yield/answer`),
and when the recording is absent the driver exits 6 itself instead of
inventing one.

## Provenance

What each harness pins, from its official source or documentation. None
defines an exit code meaning "input required" for a command it runs — which
is why this RFC exists — and each already has the pause/ask/resume loop
needed to implement §Harness.

| harness | version / commit | location | license | what it pins |
|---|---|---|---|---|
| Claude Code (Anthropic) | CLI 2.1.278 (measured `claude --version`, 2026-09-20) | code.claude.com/docs/en/hooks — "Hook input and output": PreToolUse `hookSpecificOutput.permissionDecision: "allow"\|"deny"\|"ask"`; "Exit code 2 behavior per event" | proprietary (Anthropic Commercial Terms; docs © Anthropic) | Approval is decided BEFORE the tool runs (hook or permission flow; `"ask"` defers to the user prompt). A hook's exit 2 blocks; a run tool's own non-zero exit is only an error string returned to the model — no code requests input. |
| OpenCode (sst) | v1.18.30 (tag; matches the installed binary) | `packages/opencode/src/permission/index.ts`: `Permission.ask()` blocks on a `Deferred` until `Permission.reply()` — `"once"`, `"always"`, or `"reject"` (throws `PermissionV1.RejectedError`) | MIT | The pause/ask/resume loop exists in-harness, pre-execution, promise-shaped. A child process has no way into it. |
| Codex (OpenAI) | commit `7784318b5f7fa35728d41ffa13e2a5821ebb4d75` (2026-09-15; installed codex-cli 0.155.1) | `codex-rs/protocol/src/protocol.rs`: `enum AskForApproval` (`untrusted`, `on-request`, `granular`, `never`), `EventMsg::ExecApprovalRequest` pausing the turn for a `ReviewDecision` (`Approved`, `ApprovedForSession`, …) | Apache-2.0 | Same shape: the protocol pauses the TURN for approval of a command. Under `Never`, "failures are immediately returned to the model, and never escalated" — a child's exit status carries no input-required meaning. |

PowerShell's `ShouldProcess` provenance (the producer side's model) is pinned
in `internal/agentos/confirm_test.go`'s header table.

## Bash OFF is identity

Exit 6 is an ordinary status in Bash — any program may exit 6 for its own
reasons. The handoff contract attaches only where the producer declares it:
a Bash# `@confirm` call, or a front-door verb speaking the `weavecli`
envelope. A harness recognizes a yield by the **pair** — exit 6 AND the
yield line (or `input_required` envelope) — never by the number alone. In
plain Bash and `--posix` nothing here exists: decorator syntax does not
parse, the confirm rung is not wired, `cmd/bash` structurally cannot link
`internal/agentos`, and `--what-if` is an ordinary word (fixture case 9,
`TestConfirmBashOffIsIdentity`).

## Evidence

- `test/yield/run.sh [bashy]` — the executable fixture: the real harness
  loop against a real binary, byte-diffed against the recorded transcript
  `test/yield/transcript.expected` (normal, boundary, failure and replay
  cases; the recorded human answer in `test/yield/answer`).
- `internal/agentos/confirm_test.go` — `TestConfirmHarnessLoop` (this loop,
  in process), `TestConfirmResumeAnswers` (§Resume), `TestConfirmUnattendedYields`
  and `…YieldEnvelopeUnderAgentMode` (§The contract),
  `TestConfirmBashOffIsIdentity`.
- Front-door precedent: `weavecli.ExitInputRequired` and the `input_required`
  envelope code (`coreutils/pkg/weavecli/envelope.go`), which front-door
  verbs already return.

## Non-goals

No second protocol, registry, or policy engine; no process-pausing resume
(replay is the resume); no yield from plain Bash; no machine-made answers.
Checkpoint/continuation of a yielded body (resuming PAST the already-run
prefix) is explicitly out: at-least-once replay is the contract until a real
body demonstrates the need.
