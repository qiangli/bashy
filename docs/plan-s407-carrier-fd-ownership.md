# S407.4: close background-job descriptors

Story #1947 (9f3cc23e7193) covers the Profile D awk timeout and the reported
two-descriptor growth per external background command.

## Reproduction and fix

1. Run a Linux shell regression with garbage collection disabled. Execute 200
   `/usr/bin/true & wait "$!"` jobs and count `/proc/$$/fd` before and after.
   The pre-fix shell grew from 13 to 413 descriptors.
2. In `internal/cli/carrier_unix.go`, preserve the carrier PID, then complete
   `exec.Cmd` cleanup after the terminal `wait4` observation. `cmd.Wait` closes
   parent pipes even when the status was consumed by `wait4`; `Process.Release`
   releases the Linux pidfd. Do not signal the already reaped carrier.
3. In the sh interpreter, close the `/dev/null` opened for asynchronous stdin
   after the job finishes, or immediately if starting its carrier fails.
   The sh engine fix is commit `6663c93a2` on branch `agent/s407-fd-leak` in
   `/Users/qiangli/.bashy/weave/sh-s407-fd-leak`; the exact patch is copied to
   `docs/patches/s407-sh-async-fd.diff` so the current Bashy branch carries
   the cross-repository change. It must be published and pinned in Bashy's
   `go.mod` before this Bashy branch is integrated.
4. Rerun the Linux fd test, the Bash 5.3 86-fixture gate, and a 2,000-command
   timing comparison against the previous Bashy candidate.

The Linux venue was an Ubuntu 26.04 arm64 VM with Go 1.27.2. The reproducible
script is `scripts/s407-carrier-fd-bench.sh`; both runs used the same VM,
`bashy_cert_base` build tag, `VSC_PROFILE=cert`, and `GOGC=off`:

| Build | Descriptors before → after | 2,000 awk jobs |
| --- | ---: | ---: |
| rc.13-dev `4c37c968` with published sh pin | 17 → 4,017 | 36.67 s |
| Bashy carrier fix with sh `6663c93a2` | 17 → 17 | 35.82 s |

With only the Bashy carrier fix and the currently published sh pin, a
200-command probe still grew from 17 to 217 descriptors. This isolates the
remaining `/dev/null` leak and makes the sh pin a required merge step.

The fixed loop is within 3% of the old candidate's time on this host. The
historical `ba62cd91` result (173–229 s) is for the proprietary VSC awk set,
so its wall time is not directly comparable to this synthetic loop. A new
Profile D run on that certification host remains necessary to show the awk
set is back under its 600 s cap.

The combined source passed `go test ./internal/cli -run TestCarrier -count=1`,
`go test -tags bashy_core ./cmd/bashy -run TestCoreRouteOneFileAndCommandCRUD
-count=1`, and `make test-bash` with **86 passed, 0 failed, 0 skipped, 0 timed
out**. The VM generated the locales required by the Bash 5.3 fixtures before
the final full run. Bashy's VM copy used a local module replacement to the
unpublished sh commit; this is evidence for the combined fix, not a claim that
the current published `go.mod` pin contains the sh change.

## Schedule daemon attribution

The observed `bashy schedule daemon --interval 1s` was launched explicitly by
`vsc-pcts-harness-kit/scripts/schedule-daemon.sh`, sourced by
`scripts/dev-posix-run.sh` for Profile C/D. The harness runs it as `vsctester`
and has an exit trap to stop it. Shell invocation through the `sh` or `bash`
alias enters `cli.Main` before AgentOS dispatch and has no scheduler startup
path. A shell-route regression checks both aliases with and without
`VSC_PROFILE=cert` before the separate scheduler command is exercised.

The harness still starts its daemon for Profile C/D, including the reported
certification replay. Suppressing that explicit launch requires a harness
change and coordination with its schedule tests; this Bashy change only proves
that non-interactive shell entry does not start a daemon itself.

The local `bashy gate --json` run passed all three configured checks (`go
build ./...`, `go test ./internal/... ./tools/...`, and `env make test-bash`).
Its overall verdict was `abstained` because the optional mutation probe did
not run.
