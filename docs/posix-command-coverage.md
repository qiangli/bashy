# POSIX command coverage and provider scopes

This is Bashy's routing page for the current Commands & Utilities inventory.
Do not maintain another hand-written command list here.

- Historical 2026-08-22 harness snapshot with configured TP counts (superseded
  for current ownership/counting by the generated coreutils inventory below):
  [`../../vsc-pcts-harness-kit/docs/POSIX-BASHY-COREUTILS-COVERAGE.md`](../../vsc-pcts-harness-kit/docs/POSIX-BASHY-COREUTILS-COVERAGE.md)
- Generated coreutils-side 116-row inventory:
  [`../../coreutils/docs/posix-required-commands.md`](../../coreutils/docs/posix-required-commands.md)
- Provider allocation, Ubuntu 24.04 bare-image probe, and internal/managed/system
  strategy:
  [`../../docs/posix-utility-provider-strategy.md`](../../docs/posix-utility-provider-strategy.md)
- Executed-provider and reporting contract:
  [`../../vsc-pcts-harness-kit/docs/VSC-COMMAND-PROVIDER-CATALOG.md`](../../vsc-pcts-harness-kit/docs/VSC-COMMAND-PROVIDER-CATALOG.md)

The accounting is:

| Layer | Required names supplied |
| --- | ---: |
| registered Bashy Go applets | 90 |
| shell-only entries/builtins | 14 |
| pinned external providers registered by the multicall | 12 |
| unresolved provider gaps in assembled C/D | 0 |
| configured required names | 116 |

The implementation-presence counts overlap: seven Go applets also resolve as
shell builtins, and `time` resolves as a shell keyword. Effective staged
ownership must therefore be recorded separately: 82 Go-selected names, 22
shell-selected names, and 12 provider-selected names. The assembled C/D
environment has no name-level gap.

Profile B is Bashy `sh` plus the **prepared** GNU/system provider environment;
it excludes Bashy's Go multicall. Profiles C/D place Bashy Go coreutils first.
The pristine Ubuntu image is only an input: bootstrap installs missing
packages, services, locales, users, and fixtures, and fail-closed preflight
records the provider actually resolved. Filling a bare-image package gap is
therefore useful to Profile B without becoming Bashy-coreutils implementation
credit.

There is one canonical Go applet inventory. POSIX-required is documentation
metadata, not a lean/full or certification-specific build.
