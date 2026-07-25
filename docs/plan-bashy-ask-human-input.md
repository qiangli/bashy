# Plan — `bashy ask`: human input from inside an agent session

**Status:** P0 shipped 2026-07-24. Design of record lives in the umbrella at
`docs/bashy-ask-human-input-design.md`; this file is the bashy-side copy required
by the repo's "save all implementation plans in docs/" rule.

## What it is

A command an agent runs to get an ad-hoc value from the HUMAN, over a channel the
agent does not own, returning a path rather than the value.

    bashy ask --prompt "GitHub PAT" --name GH_PAT
    /Users/you/.bashy/ask/a7f3c1d2.../value

Engines live in coreutils (`pkg/ctty` = reach the human; `pkg/ask` = the request,
the rendezvous, the sinks). bashy contributes only the verb wiring.

## Why it exists

A command run by an agentic CLI does not own its stdin or stdout — both are pipes
owned by the harness. A prompt written there is seen by nobody; a value written
there enters the transcript, the model context, and the provider's logs. The
standing workaround was writing the value to `/tmp/x`.

Measured on macOS from inside a Claude Code tool call: the harness `setsid`s its
children, so `/dev/tty` returns ENXIO and the obvious implementation cannot work.
A GUI askpass does work from a tty-less child. Hence a ladder:
controlling terminal → GUI askpass → out-of-band rendezvous.

## bashy-side changes (the four registration points)

1. `internal/agentos/agentos.go` — `"ask"` in `alwaysShimVerbs`.
2. `internal/agentos/agentos.go` — `case "ask"` in the `Dispatch` switch.
3. `internal/agentos/commands.go` — `verbSynopsis["ask"]` (test-enforced).
4. `internal/agentos/commands_e2e_test.go` — `"ask"` in the `native` set, so the
   3-OS e2e dispatch gate runs `bashy ask --help` on every platform.

Plus the atlas entry in `coreutils/pkg/atlas/atlas.go`:
`Stage: StageCross, Group: GroupPlatform, Caps: {CapJSON}`, effects `cred`+`write`.
**No `CapNeedsNetwork`, no `CapNeedsPairing`** — that is the whole reason this is
its own verb rather than `bashy secrets input`. Caps say what a verb REQUIRES;
`secrets` genuinely requires a paired cloudbox because every subcommand is a vault
RPC, and prompting the human at this keyboard requires neither.

## Verifying a change here

    go test ./pkg/ask/ ./pkg/ctty/          # in ../coreutils
    go test ./internal/agentos              # in bashy
    BASHY_TELEMETRY_QUIET=1 go test -tags e2e -run TestE2EAllListedCommandsDispatch ./internal/agentos

The `BASHY_TELEMETRY_QUIET=1` matters only on a host with OTEL spooling configured:
the telemetry banner otherwise lands in the captured output the e2e gate decodes as
JSON. CI has no endpoint, so it does not need it.

Live smoke from inside an agent session:

    BASHY_ASK_DIR=/tmp/asktest bashy ask --channel rendezvous --prompt "test" --name T &
    BASHY_ASK_DIR=/tmp/asktest bashy ask ls
    printf 'value' | BASHY_ASK_DIR=/tmp/asktest bashy ask answer <id>

Then confirm the value file is 0600 and the secret appears in neither the
requester's stdout nor its stderr.
