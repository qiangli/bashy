# OS-backed job carriers: real kernel PIDs for `$!`

**Status: shipped** (2026-08-02). Engine seam: `interp.WithJobCarrier` in the
`../sh` sibling (`interp/carrier.go`, landed at `34bd799e`); Bashy host wiring:
`internal/cli/carrier*.go`.

## Why

The interpreter runs asynchronous lists (`cmd &`) as goroutines, so a job made
only of builtins or compound commands has no OS process and `$!` fell back to
the synthetic `g<N>` handle. That is invisible to every external tool — and it
invalidated a live VSC-PCTS run: the toolkit stores `$!` in `tet_context`
(observed: `tet_context=47066`) and `tetapi.sh` does *integer* comparisons on
it; `cmd/bash` returning `g1` broke the suite before a single assertion ran.

## What Bashy wires

`internal/cli/newRunner` — shared by **both** `cmd/bash` (the certification
SUT) and `cmd/bashy` — passes `interp.WithJobCarrier` an OS-backed carrier:

- **One re-exec of the current executable per background job**, in helper mode
  (`argv = [argv0, "--bashy-job-carrier"]`). The helper is intercepted before
  process groups, AgentOS dispatch, flag parsing, startup files and telemetry
  (`cli.Main` first line; `cmd/bashy` additionally intercepts before
  `telemetry.Init`), executes no shell or user code, and runs with an empty
  environment.
- **Lifetime is the parent-owned stdin pipe**: the helper exits on EOF, so a
  parent that dies without reaping cannot leak carriers. `Terminate` (close
  pipe + kill) is idempotent via `sync.Once` and race-safe against `Wait`.
- **Real job-control identity on Unix**: each carrier gets its own process
  group (`Setpgid`), keeps its default signal dispositions (the helper installs
  no handlers), and `Wait` maps a signal death to its number from the wait
  status — the engine relays it per the job's own trap/ignore/default
  disposition, so an external `kill -TERM $!` on a pure-builtin job yields
  `wait` status 143, `-KILL` 137.
- **Standalone CLI runner only.** `NewSessionRunner` (warm `bashy serve`) and
  any embedded/deterministic runner never consult the seam — they must not
  spawn processes their host did not ask for. The engine itself ignores the
  option under `set -o dryrun` and deterministic mode.
- **A basic carrier on Windows.** A carrier is pure process identity, and
  Windows has that: `CreateProcess` gives a real pid, `OpenProcess` answers
  `kill -0`, and the engine's `kill` already speaks the Windows dialect of
  every signal. What Windows lacks is the *live proxy* — `os/exec` rejects
  `ExtraFiles`, so there is no private descriptor to relay catchable signals
  over — so `carrier_windows.go` implements the plain `interp.JobCarrier`
  contract instead: the carrier keeps its default dispositions, an external
  `kill` ends it, and the engine relays that signal to the job when it reaps
  it (trap runs, ignore holds, otherwise 128+signal). The signal number comes
  from the exit code, which Windows kills carry instead of a wait status:
  `interp.SignalMarkerExitCode` writes the marker, `SignalFromMarkerExitCode`
  reads it back. Each carrier is created with `CREATE_NEW_PROCESS_GROUP`, the
  local counterpart of Unix's `Setpgid`, so a console `^C` aimed at the
  foreground job does not strike carriers.
- **Fail closed, never `g<N>`, on unsupported platforms**: Plan9/JS
  have no platform carrier; bash mode there keeps the legacy synthetic handles,
  but POSIX/sh mode (`--posix`, `-o posix`, argv0 `sh`) wires a carrier whose
  `StartCarrier` always errors, so each background job fails with a diagnostic
  and exit 1 — strict process semantics are never silently faked. Engine-side
  startup-failure/nonpositive-PID paths fail closed the same way (sibling unit
  coverage; Bashy's injection seam `newCLIJobCarrier` is test-covered in
  `internal/cli/carrier_test.go`).

## Verification

`internal/cli/carrier_unix_test.go` drives the built `bin/bash` end to end:
the exact certification shape (`(:)& p=$!; case $p in *[!0-9]*|"") exit 90;;
esac; /bin/kill -0 "$p"; wait "$p"` → exit 0), deterministic external
`kill -0` liveness, TERM→143 / KILL→137 on pure-builtin async compounds,
carrier reap on natural completion (no leaked PIDs), and helper-mode
EOF/signal lifecycle. The bash 5.3 serial gate stays 86/86 with carriers on.
`internal/cli/carrier_windows_test.go` pins the Windows carrier: a live,
probeable pid; the signal-marker exit code round trip (TERM→15, USR1→30);
stdin EOF alone ending the helper; an idempotent `Terminate`; and the
end-to-end regression — a POSIX-mode `{ echo ran; } &` that used to fail
closed with "unsupported on windows", which is what the `jobs` and `posixexp`
fixtures saw instead of their output.

One trade recorded by the engine contract: a job that *survives* a relayed
signal (trapped or ignored) has lost its carrier and with it its external
kernel identity; `wait`/`jobs` still resolve it.
