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
prerequisites. Only the census failures above have a completed exact-baseline
comparison. The remaining failures are unresolved; they cannot be described as
baseline-only or passing. Raw outputs remain in the manager's private evidence
bundle. Owned test artifacts were reclaimed without deleting unrelated caches.

A subsequent native Windows focused run passed all seven selected packages after
portable shell fixtures and archive metadata were corrected. The latest targeted
rerun passed first-activation undo, legacy attribution, presentation and schema
checks, but exposed an invalid TODO directory for actor identities containing a
colon. That failure remains open while its path handling is repaired.

Fresh Linux checks of yoke `9da6d256` and Bashy `0fc8d967` passed all `shape/...`
packages and `src`. The complete earlier ratchet remains failed. Three failures
in DAG/source/startup telemetry passed on unchanged source after correcting the
runner PATH and moving temporary files outside HOME. The schema golden,
presentation suffix and legacy attribution defects have separate fixes; their
passing focused reruns do not make the earlier full ratchet pass.

The original Linux Bashy general target failed its startup budget under load
before reaching the Go suite. The Bash 5.3 gate was **80/86, failed**: several
fixtures require an unprivileged user or locales absent from the test image.
Corrected environment runs and baseline comparisons are pending. Expectations
and startup budgets have not been weakened.

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
