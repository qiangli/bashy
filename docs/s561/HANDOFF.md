# Sprint 561 handoff (urgent pickup note)

Written 2026-11-04 by the S561 manager (`claude-opus5.5`, interactive session).
Operator goal: **finish S561 cleanly with no follow-ups**, so later sprints are
unblocked. Private host and harness specifics are in the local sprint card
continuity (`bashy sprint show 561`), not here.

## Take the seat

`bashy sprint take 561 --owner <your-registered-agent>`; save the printed lease
token privately and export `BASHY_SPRINT_LEASE_TOKEN`. `sprint take`/`assign`
can run 5–20+ minutes CPU-bound; run them in the background. Leases go stale
after ~30 min without a checkpoint. Read mail with
`bashy inbox --as <name> --peek` (a bare shared name may be ambiguous).

## State (8/10 stories closed)

Closed: A1 (profile + manifest), A1b (boundary audit), A2 (issue list), A3
(+A3a go-test fixes, A3b CLI `-c` option scan + sh self-signal, A3c utility
triage), A5 (content-addressed evidence identity). Evidence: `docs/s561/`.

**Frozen candidate** (pushed): bashy `f1bcd7117a93cbc3c40a26010bf607cac251d682`,
sh `74586fde5f2497fa89d83b142438daf7f00ef05c` (branch `s561/a3b-self-signal`),
coreutils pin `211789fa` (code-identical to pushed `601546e1`), umbrella
`dfbf104a`. Installed on the dev host (`make install` + `~/bin/sh`); prior
`sh` kept as `~/bin/sh.previous-s561-rc23`. CI on `f1bcd711`: Linux and macOS
green; Windows has 5 failing `internal/agentos` tests (see A3d).

**Evidence identity**: `sh scripts/cert-identity-check.sh
docs/s561/candidate-manifest.json HEAD` must print `SAME` for both cert builds;
then the candidate's evidence still applies. Any `CHANGED` means rerun.

Freeze agreement: the S562 (now `codex-gpt6-sol`) and S563 managers make no
bashy `go.mod`/pin/source changes until S561 announces the licensed run done;
no one pushes bashy `main` without the other managers' ACK.

## Remaining work (operator-approved 2026-11-04)

1. **A3d — Windows**: fix the 5 Windows-only `internal/agentos` failures
   (`TestPodmanConfOverrideLinuxPaths`, `TestRepairUnitsMaterializeAsBashSharpShims`
   — "go shim must run the pinned bashy's go", `TestRepairPosixLinksNeverShadowWrapperUnits`
   — "wrapper sleep must stay the authoritative Bash# unit",
   `TestRepairSuiteTimeoutKilledNeverPasses` and
   `TestRepairProfileKilledSuiteBlocksPassAndCompletion` — TempDir cleanup
   "file being used by another process" after a killed suite). No skips or
   weakened assertions. Verify on the Windows test host (coordinate its claim
   with the S562 manager) and in CI. These packages are outside both cert
   builds, so the identity check must stay `SAME`.
2. **A4a — public heavy lanes** on the frozen candidate: full
   `GOWORK=off go test ./...`, `make dist` (12 binaries, record sha256),
   `make test-bash-container`. The first attempt failed on disk exhaustion
   (results in `docs/s561/a4a-results.md`). Operator allows the dev box, the
   Windows/Linux LAN hosts, or disposable droplets; disk now has headroom.
3. **A4 — licensed run** (approved by the operator): one Profile D run
   (all 117 sets incl. the 493-TP shell scenario, Bashy Go utilities, zero
   external providers) plus the Profile B 493-TP shell release gate, both on
   disposable hosts via the harness kit lifecycle. Blocker to clear first:
   the harness transfer validator needs a published approval commit whose
   gitlinks equal the record — umbrella `main` carries S562's `sh` gitlink, so
   publish a dedicated umbrella approval tip with bashy `f1bcd711`, sh
   `74586fde`, coreutils `601546e1`, plus the private approval record (see
   card continuity). Then reconcile results against prior green coverage and
   close A4 with the handoff pack (identities, commands, results, residuals).
4. **Push** remaining local docs/scripts (closure `SAME`) after S563's ACK
   (S562 ACKed).

Known non-pass results that need dispositions, not code: 53 empty-symlink
setup failures, 9 INSPECT, `man:4` threshold and four `more` EOF/editor
results (VSC authority questions). The `find -exec`/`env COMMAND` NO-list is an
operator decision.

## After S561

`docs/plan-layer-delineation.md` (A Classic / B Bash# / C Yoke; removing Bash#
from the Classic `bash` binary is approved) starts only after the licensed run.
