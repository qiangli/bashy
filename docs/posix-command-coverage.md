# POSIX command coverage and provider scopes

This is Bashy's routing page for the current Commands & Utilities inventory.
Do not maintain another hand-written command list here.

## Certification scope — read this first

The certification claim is the **Sprint 341 base Shell and Utilities scope**
(`../../vsc-pcts-harness-kit/docs/SPRINT-341-BASE-SCOPE.md`, enforced by
`../../vsc-pcts-harness-kit/docs/PROFILE-D-ONLY-CERTIFICATION-HOST.md`). It is
already decided; do not re-open it as a sprint decision.

- **Required:** the Bashy shell plus the **102 mandatory VSC utility targets**,
  routed through exactly **92 Go applets and 10 Bashy shell commands, with zero
  external utility providers**. `gencat` (mandatory, absent from VSC's
  116-name target list) also routes to Go and is proven separately. `man` is a
  mandatory target and routes to Go.
- **Optional, outside the claim:** the 14 option-feature targets `ar`, `bg`,
  `ctags`, `ex`, `fc`, `fg`, `jobs`, `localedef`, `make`, `more`, `nm`,
  `strip`, `talk`, `vi`. They stay in the full inventory but do not gate the
  claim.
- The certified product is **pure Go**. No external provider, host utility or
  new utility implementation is required for the claim; planning that proposes
  one (or a "shell + N utilities" profile choice) is scope creep. The six
  remaining external-provider names below are all in the optional set.

The 116-name inventory is the complete shipping catalog, not the required
denominator. The scored denominator comes from the licensed configured suite
and its official report.

## Inventory sources

- Generated coreutils-side 116-row inventory:
  [`../../coreutils/docs/posix-required-commands.md`](../../coreutils/docs/posix-required-commands.md)
- Interface evidence ledger:
  [`../../coreutils/docs/posix-required-command-interfaces.md`](../../coreutils/docs/posix-required-command-interfaces.md)
- External-provider mechanism (optional names only):
  [`../../coreutils/docs/posix-external-providers.md`](../../coreutils/docs/posix-external-providers.md)
- Executed-provider and reporting contract:
  [`../../vsc-pcts-harness-kit/docs/VSC-COMMAND-PROVIDER-CATALOG.md`](../../vsc-pcts-harness-kit/docs/VSC-COMMAND-PROVIDER-CATALOG.md)
- Historical 2026-08-22 harness snapshot (superseded for ownership/counting):
  [`../../vsc-pcts-harness-kit/docs/POSIX-BASHY-COREUTILS-COVERAGE.md`](../../vsc-pcts-harness-kit/docs/POSIX-BASHY-COREUTILS-COVERAGE.md)

## Accounting (generated inventory, coreutils `main`)

| Layer | Full 116 inventory | 102 base targets |
| --- | ---: | ---: |
| registered Bashy Go applets | 96 | 92 |
| shell-only entries/builtins | 14 | 10 |
| pinned external providers (`ar ctags ex nm strip vi`) | 6 | 0 |

Effective ownership in the interface ledger is 88 Go, 22 shell and 6 provider
(seven Go applets also resolve as shell builtins, and `time` is a shell
keyword). Recount from the generated TSVs rather than copying these numbers
forward.

Bashy's agent-mode command atlas (`TestDocumentedExternalCount`) separately
lists POSIX-named catalog entries whose origin is external. That catalog
describes what agent mode can provision; it is not the certification route and
does not change the zero-provider base profile.

Availability and ownership are not conformance evidence. The interface ledger
records evidence state separately, and registry ownership passing never means a
name's full flag/option/operand conformance is proven.

Profile B is Bashy `sh` plus the **prepared** GNU/system provider environment
(a diagnostic control); it excludes Bashy's Go multicall. Profiles C/D place
Bashy Go coreutils first; Profile D is the integrated certification candidate.
Filling a bare-image package gap helps Profile B without becoming
Bashy-coreutils implementation credit.

There is one canonical Go applet inventory. POSIX-required is documentation
metadata, not a lean/full or certification-specific build.
