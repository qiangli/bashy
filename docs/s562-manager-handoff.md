# Sprint 562 manager handoff

Warm handoff, 2026-11-04, from `claude-opus5.5-s562` to `codex-gpt6-sol`
(operator's choice). Plan of record: `plan-s562-master-execution.md`. The card,
its stories and the sprint thread are authoritative; this note is the resume
brief behind them.

## State

- Stories: **E2 accepted** (1/10). Everything else open.
- `bashsharp-tests` main `1960177`: shared runner (E1) with build mode, all gate
  normalizers incl. compound lists, per-run toolchain provisioning (incl. the Go
  version the fence resolves), run-cache cleanup, ratchet; Python corpus required
  on linux (23/23 on linux/amd64 and darwin/arm64).
- `sh` master `5ce099d0` (local, **unpushed**): S562 B3 batches 1-4 (Go void
  exports, typed-assign opt-in error, unknown-function resolution order, Go exit
  status Go-only, Go unbridgeable exports module-internal, Python handle method
  calls) + S561's `74586fde` merged in.
- bashy main: S562 docs only; bashy's sh pin is S561's (`74586fde`).

## In flight (bashsharp-tests weave unless noted)

| Run | Agent | Story | Next manager action |
| --- | --- | --- | --- |
| #40 | muse-spark1.2 | E5 | Reviewed + validated on main. Merge only after the Linux fenced-Go measurement (below) passes; then lock counts, flip tour-fenced/examples-fenced to required, close E5 and B3 (B3's Go fixes are proven by that run). |
| #35 | claude-opus5 | E3 | Resumed; finishing TS/Rust counts after the clock fix (2 date cases are env-dependent; convert to `wallclock` compare in #38). |
| #38 | claude-opus5 (queued) | E3 | Pin the Rust toolchain (`rust-toolchain.toml`) — diagnostics drift with floating `stable`. Add the wallclock compares. |
| #28 | claude-fable5.1 | E4 | Long revision (C/C++, C#, PowerShell; fragment challenge). Resume on runtime-cap kills with `weave start --run 28 --resume`. Consider splitting C# + PowerShell to a second agent. |
| sh #9 | codex-gpt5.6-sol | E6 | Python iterators (design note first). Callbacks next: design note, manager review, then code. |

E1 can close now: runner, validator, ratchet and hooks merged; remaining E1
items are done. Accept it after one look at `tools/fence-corpus/README.md`.

## How to verify (lessons paid for)

- Merge rule: run the fatal aggregate on the **merged** tree before every merge.
- Verify commands must set `BASHY_BIN`; use a fresh `FENCE_TOOLCHAIN_DIR` per run
  (one dir cannot be shared concurrently, and a reused dir can hold a stale SDK).
- Corpora with sh-fix dependencies need a **candidate** bashy: `go build ./cmd/bashy`
  from `bashy/` with the umbrella go.work (live `sh`), or cross-compile
  `CGO_ENABLED=0 GOOS=linux GOARCH=amd64`. Record bashy + sh SHAs and sha256.
- Hosts (by role; claims held by the outgoing manager, transfer them):
  the **Windows host** (32 cores, ~1.5 TB) — Linux via its WSL Ubuntu 24.04
  distro, native Windows for the C#/PowerShell leg; the **macOS test host**;
  never full corpora on the dev box. Ephemeral Linux droplets are allowed when
  needed (operator).
- Known baseline failures (report, do not repair): `sh` `TestS243UnchangedIssue43164`,
  `TestRunnerRunConfirm/0049|0856|0159`; flake `TestSprint162BashPPFIFOAfterTask/procsub-external-control`.
- Workers: Claude-based workers sometimes end their turn while their own jobs
  run; resume them. Story trailers: `Story: #<seq>` plus `Story-ID:`. One story,
  one owner: split work into a new story for parallel agents.

## Agreements with S561

No bashy go.mod/pin/source changes from S562 during S561's A4 freeze (they
announce start/end). Neither manager pushes bashy main without the other's ACK;
the operator approves pushes. S561's certification candidate pins sh
`6663c93a+74586fde` only. S562 bumps bashy's sh pin only after S561 says so.

## Open bashy defects reported to S563

Seat matching (a clone's watch/peek/send seats the parent agent); stale lease
although checkpoints say "refreshed", and `sprint take` hanging; leaked test
temp trees (`internal/agentos/main_test.go` `bashy-agentos-commands-*`).
Workaround: no `--watch`; read with `inbox --as NAME`; reply on sprint threads.

## Remaining after the E-stories

B1-B4: fence × platform × provider matrix, baselines, repairs, frozen candidate
verification and the bashy sh pin bump — after S561's freeze ends.
