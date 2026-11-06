# Sprint 564 validation evidence

## H2 atlas census baseline (2026-11-05)

A fresh, narrow comparison on the local macOS development-role environment
reproduced the existing census failures with isolated source archives. Both
variants used the same pinned dependency snapshot and coreutils source:
`eb3f31c0df7d8ed5180549d27f55571d1d50d2bf`.

| Variant | Exact yoke source | Result |
| --- | --- | --- |
| Before H2 | `2c22ccb619d432d4f3b71c57e6a65c80c4e3af64` | FAIL, exit 1 |
| H2 candidate | `6f6608d069083fb46a675814ecd670b1e6d96f3c` | FAIL, exit 1 |

Each variant ran only the existing test:

```sh
GOFLAGS=-mod=readonly go test ./pkg/atlas \
    -run '^TestCensusCoversEveryPackage$' -count=1 -v
```

`GOWORK` selected each isolated variant's workspace with the same dependency
sources. The supplied validation snapshot was read without modification; no
atlas rows, test expectations or known-failure records were changed.

Both runs reported exactly these failures at `packages_test.go:77`:

```text
pkg/nativepath has no atlas census entry.
pkg/nfthelper has no atlas census entry.
```

Both missing package entries belong to yoke. The
[existing census test](https://github.com/qiangli/yoke/blob/2c22ccb619d432d4f3b71c57e6a65c80c4e3af64/pkg/atlas/packages_test.go#L66)
checks the union of the yoke and selected coreutils package directories.
The two failures are unchanged from the exact baseline; **the test is not
passing**. This comparison establishes no regression for these two identities,
not a clean atlas suite or full Sprint 564 acceptance.

The manager retains the two source manifests, raw logs and machine-readable
comparison in the local Sprint 564 evidence bundle, identified as
`census-baseline` and `census-h2`. Test ownership, role-based host context and
remaining acceptance follow the master's
[evidence and completion rules](plan-s564-master-execution.md#evidence-and-completion).
Other operating systems and the remaining H5/H7 checks need their own matching
candidate evidence.

## Native shaping candidate (2026-11-05)

The shared native implementation is committed in yoke `06d844da`; the CLI adapter
is committed in Bashy `cc878fc1`. These are implementation candidates, not sprint
acceptance or an installed release. Focused workspace checks passed:

```sh
go test ./yoke/pkg/shape ./yoke/pkg/src ./yoke/pkg/shape/verify \
  ./yoke/pkg/fleet ./bashy/internal/agentos \
  -run 'TestNativeSubmit|TestShape|TestProgram|TestNativeGo|TestService|TestAppShaping|TestSlot' \
  -count=1
```

This includes native proposal verification refresh followed by the ordinary merge
gate, policy/artifact drift refusal, side-slot recovery, and retained-controller
routing. Earlier focused web lifecycle and browser submission checks also passed;
they do not establish the real-agent matrix.

The first macOS test-host full run did not pass. The existing yoke ratchet and
cross-vet gates failed; the product and Bash gates encountered insufficient disk
space, and the general test target also lacked its Node/package-manager
prerequisites. The later Linux cross-vet comparison below attributes the
matching platform diagnostics to the baseline. Other failures need their own
resolution or comparison; neither full gate is passing. Raw outputs remain in the manager's private evidence
bundle. Owned test artifacts were reclaimed without deleting unrelated caches.

A subsequent native Windows focused run passed all seven selected packages after
portable shell fixtures and archive metadata were corrected. The latest targeted
rerun passed first-activation undo, legacy attribution, presentation and schema
checks, but exposed an invalid TODO directory for actor identities containing a
colon. A bounded repair now keeps the actor identity intact and uses a collision-safe
Windows directory locator shared with ordinary TODO resolution. A native
Windows overlay rerun passed the exact Genie test, locator/API tests and
tracking attribution tests; it reused the recorded shell checker rather than
claiming a new product build.

Fresh Linux checks of yoke `9da6d256` and Bashy `0fc8d967` passed all `shape/...`
packages, `src`, `webconsole` and `fleet`. Focused CLI checks also passed.
The complete earlier ratchet remains failed. Three failures
in DAG/source/startup telemetry passed on unchanged source after correcting the
runner PATH and moving temporary files outside HOME. The schema golden,
presentation suffix and legacy attribution defects have separate fixes; their
passing focused reruns do not make the earlier full ratchet pass.

The original Linux Bashy general target failed its startup budget under load
before reaching the Go suite. The Bash 5.3 gate was **80/86, failed**: several
fixtures require an unprivileged user or locales absent from the test image.
A corrected run using an unprivileged fixture owner, the matching launcher and
companion binary, and seven generated locale providers passed the canonical
fixture loop **86/86**, with no skips or timeouts. The original failure and a
separate staging failure remain preserved. Startup-budget baseline comparisons
and the general test target remain pending. Expectations and startup budgets
have not been weakened.

The first real Codex/CLI greeting lifecycle verified, previewed and activated the
candidate with real Chrome assertions and independent-principal isolation. Its
first undo failed because the implicit release had not been retained as the
previous version. Explicit restoration recovered the original greeting; ordinary
proposal submission then completed with an unstarted review run. This cell is
**failed**, pending a fresh first-activation test of the separate undo repair.
It does not establish any other agent/interface cell.

Remaining acceptance includes complete Linux/macOS/native Windows gates,
original console regression, actual Klatch image/data activation, Bashy
self-shaping with Sprint 570 H1, and all 24 real-agent/interface cells.
Sprint 570 H1 is still backlog; a help-text demonstration cannot replace it.

## Cross-vet baseline comparison

The complete Linux-host cross-vet run was repeated against the pre-shaping
sources: yoke `2c22ccb619d432d4f3b71c57e6a65c80c4e3af64` and Bashy
`5a9da7c63b390fe5256a0e37a3387b9ae80644f1`, with the other nineteen source
inputs unchanged and Go 1.27.2. Both candidate and baseline reported Windows,
Darwin and AIX failure and Linux success. Their normalized diagnostic sets
matched exactly, with no candidate-only or baseline-only reasons: Windows
GPGME/Podman build constraints and resource-limit symbols, Darwin libkrun build
constraints, and AIX file-lock/Unix symbols. Raw logs and all source manifests
remain in the manager's private evidence bundle. This establishes unchanged
cross-vet failures; it does not establish a passing cross-vet gate.

## Fresh real-agent greeting retest

A fresh H7 A / Codex / CLI cell passed independent manager review on native
macOS arm64. The immutable binary was built on Linux for Darwin arm64 with
CGO disabled, Bashy `0fc8d9675c2645038dfd97ffb76e4c41aa6fdd79`, yoke
`9da6d2566a72ed63605900586f9d2479fe540850`, and clean version-control metadata.
Its SHA-256 is
`7bb7a793c8589a5e55a13add33799b0ea9e11bab0217a66f13a161710f0055d1`.
The real agent was Codex 0.157.1 using `gpt-6-sol`.

The original fixture commit `8654cf6a389a2fb3640271b6136ef31641bcc5d2`
was reused in a new instance with no prior activation history. Six actual Chrome
checkpoints covered baseline, candidate preview, unchanged active app, approved
app, independent second principal and first undo. Undo restored the exact fixture
revision and greeting. Verifier evidence passed; the reviewed final proposal
`90a7ace02c48c014fdc7bd4abb96b9d1c5dfb677` changes only the greeting file,
contains the expected shaping attribution, and has an open ordinary source PR
and an unstarted review run. The earlier failed cell remains preserved.
This is **one of 24 cells**, not H7 acceptance.

A separate Claude/CLI cell was prepared and its real Chrome baseline passed,
but Claude Code 2.1.284 using the registered `claude-opus-5-5` returned HTTP 429
with a weekly quota limit before any lifecycle tool use. That cell is
**BLOCKED**, not passed; no alternate model or retry was substituted.

The same immutable binary and fixture also passed manager review for H7 A /
agy / CLI and H7 A / Codex / REST. agy 1.5.1 used the registered
`gemini-3.8-flash-high`; its final proposal is
`72392f8e430eb4e144017b049167e9085585da89`. The Codex REST run used actual
paired HTTP requests, natural-wish generation, six Chrome checkpoints and
proposal `111c1e5799f5219335dbfabd7d3fa963ba82fc5e`. In both cases first undo
restored the exact fixture revision, the second principal stayed unchanged,
and submission produced an ordinary unstarted review run. The REST candidate
recorded its delegate executable and `agent_identity`; optional `agent_receipt`
was absent. An earlier manager lookup used incorrect field names and has been
corrected. This brings reviewed passing coverage to
**3 of 24 cells**. Other cells remain open.

The Codex MCP cell is blocked: per-invocation server configuration was recognized
by the client's MCP configuration listing, but the real agent execution did not
expose the selected server. Attempts are retained, with no successful lifecycle
calls or substitute interface. A later test found and repaired a harness path
normalization error; earlier absence of tool traffic did not isolate a client
limitation. An independent operator protocol preflight initialized the actual
Bashy MCP server and discovered `shape`, but status was denied by the default
capability guard. It is not a successful lifecycle or agent E2E pass. The isolated
MCP fixture now grants the documented destroy/spend effects required by the
aggregate shaping tool; actual agent qualification is being rerun.

## Actual Klatch validation in progress

The disposable Linux service serves the pinned real Klatch source and immutable
base image. The initial shallow checkout exposed a local transport limitation;
a separate repair now refuses shallow sources before candidate/tracking writes
with instructions for an explicit full-history fetch. The operator completed
history while preserving the exact source commit/tree and base image identity.
The retained earlier candidate remains failed.

The next real candidate built and passed all seventeen named Go tests, but the
native accounting rejected a package-level `[no test files]` result. A separate
repair now accepts only exact empty-package metadata recorded from the same
baseline listing and candidate output. Named skips, unknown package skips,
missing checks, failures and early exit still refuse verification. Real Go
1.27.2 multi-package and focused lifecycle tests pass. Actual Klatch revalidation,
preview, activation, data preservation and rollback remain pending.

A fresh Muse/CLI retest also passed manager review, bringing coverage to
**4 of 24 cells**. Muse 1.4.3-R5018.1 used the real Meta `muse-spark-1.3`
provider. Its per-run network setting enabled the external Chrome observer while
retaining the filesystem sandbox; the earlier browser-blocked run remains
blocked. The fresh cell records separate baseline, preview, active-before-approval,
approved, second-principal and undo checkpoints. Proposal
`f90afb3d87f74906364452f299bb6e24d1bb7c78` changes only the greeting file
and has an ordinary unstarted review run. Manager review uses the original
agent event outputs for transitions; later read-only captures are labeled as
post-run inspection.

| H7 A agent | CLI | MCP | REST |
| --- | --- | --- | --- |
| Codex | PASS | BLOCKED, client did not expose configured server | PASS |
| Claude | BLOCKED, provider weekly quota | NOT RUN, provider prerequisite blocked | NOT RUN, provider prerequisite blocked |
| Muse | PASS, fresh network-enabled retest | PASS | PASS |
| agy | PASS | IN PROGRESS | PASS |

The first H7 B CLI run stopped at a test-harness entry error before build or
tests ran. Its failed evidence is retained; the corrected harness is being
retried against the same candidate and retained release. Required accompanying
app/UI self-shaping proof is still missing. No H7 B or full-matrix acceptance is claimed.

## Startup gate remains unresolved

The first paired quiet-host hard gate passed on baseline and failed on the
candidate. Three additional paired traces showed overlapping totals (baseline
13.811–15.450 ms; candidate 14.643–19.809 ms), with the same 390 initialized
packages and no added or removed package identities. Differences were spread
across existing dependencies; these samples do not isolate a shaping source
regression or prove environment-only causation. The unchanged 15 ms budget
remains enforced. The general target's own startup check and remaining general
suite results are pending; earlier failures are preserved.

The fresh native Windows source run on yoke `57c0b51f` and Bashy `482b5c71`
passed seventeen selected tests across six packages, including the real Go
multi-package accounting case, Windows owner locator, shallow-source refusal,
first undo, Genie/presentation and schema golden. The existing shell-checker
artifact was reused and identified separately; no fresh installed Windows
product qualification is inferred. A Windows symlink test's existing early
return is excluded from the claimed platform evidence.

The later Linux general target passed its build-tag/container/static-scratch
prerequisites but failed its real scratch-container smoke because the test PATH
omitted system sbin directories. An unchanged, previously working artifact
reproduced that failure with the limited PATH and passed with the full standard
PATH, where packet-filter helpers resolve. This establishes the environment
cause for that smoke failure, not a passing general target. The direct Go suite
is running separately; a full-target rerun still requires the corrected PATH.

The real Muse/MCP greeting cell passed manager review after the harness path
fix and explicit test-server effect grants. Actual `initialize`, `tools/list`
and successful `shape` tool calls cover the full lifecycle; six real Chrome
checkpoints and the normal proposal/unstarted run were independently reviewed.
Proposal `9695900cfcafa65a0c88c0b238cdae15cfc82eba` changes only the greeting.
Earlier startup/capability denials and an initial help-argument error remain
retained. This brings reviewed coverage to **5/24 cells**, spanning all three
interfaces. The matrix's Muse/MCP entry is now PASS; Codex/MCP remains blocked
in the tested runtime after the corrected harness, without an isolated cause.

Actual Klatch revalidation then exposed an argv guard bug: `go -C net/server`
caused the module directory itself to be frozen as though it were a gate file.
The tests' generated database was consequently refused after named tests passed.
A narrow repair keeps argv-derived file guards and explicit protected directories,
while excluding directory/package selectors. A real Go module regression changes
production code inside the selected directory and creates runtime data; tracked
test, fixture and dependency tampering still fails. The failed Klatch run is
retained, and a fresh-controller rerun remains pending.


The real Muse/REST and agy/REST greeting cells also passed manager review,
including original HTTP lifecycle records, six actual Chrome checkpoints,
first undo and ordinary unstarted proposals. Reviewed passing coverage is now
**7/24 cells**. The AGY MCP cell is being exercised with an isolated native
client profile; authentication and actual tool calls remain to be established.

The next actual Klatch run passed its build and named tests but failed the
source dependency fingerprint because the default verifier placed Go caches
under its source-contained HOME. The repair allocates an owned runtime home,
temporary directory and caches outside both source snapshots, shared across
the baseline listing, build and tests. Dependency fingerprinting retains all
source manifests; no generated-directory exemption was added. Focused real
runner and regression tests pass. Klatch activation remains unverified until
the refreshed controller completes the real lifecycle.

The direct Linux Go suite finished failed. The two shaping command-name
inventory omissions are corrected. Four deterministic failures reproduced
against the frozen baseline; aggregate timeouts in two packages remain
unresolved. Neither that suite nor the full Make target is reported passing.


The refreshed Klatch controller passed its build, all seventeen named Go tests
and source dependency checks. Disposable health then failed before preview
creation: the real catalog wraps its missing-unit sentinel, whereas preview
code used a filesystem-only absence check. Four runtime-load checks now unwrap
the sentinel with `errors.Is`. A regression using the real catalog covers the
verification probe, preview creation, missing-preview recovery and owned cleanup.
The failed real-run seal remains preserved; a fresh-controller lifecycle retry
is required before service acceptance.

AGY MCP authentication now works through a narrow reference to the existing
native login store. Its default headless policy separately denied discovery
commands and a discovery tool before lifecycle calls. Those blocked attempts
remain recorded; supported session-only grants are limited to exact schema
reads and the selected server's discovery/shape tools. No completed lifecycle
or cell pass is inferred from authentication, tool listing or process exit.
