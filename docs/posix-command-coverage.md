# POSIX command coverage and provider scopes

This is Bashy's routing page for the current Commands & Utilities inventory.
Do not maintain another hand-written command list here.

- Canonical 116-row inventory with configured TP counts, Bashy Go applet
  presence, shell builtins, the 26 assembled gaps, and 71 extras:
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
| canonical Bashy Go applets | 76 |
| shell builtins plus `sh` not already credited above | 14 |
| external-provider gaps in assembled C/D | 26 |
| configured required names | 116 |

Coreutils alone is absent for 40 names. The assembled Bashy environment is
missing 26 internally because the shell supplies 14 of those 40.

Profile B is Bashy `sh` plus the **prepared** GNU/system provider environment;
it excludes Bashy's Go multicall. Profiles C/D place Bashy Go coreutils first.
The pristine Ubuntu image is only an input: bootstrap installs missing
packages, services, locales, users, and fixtures, and fail-closed preflight
records the provider actually resolved. Filling a bare-image package gap is
therefore useful to Profile B without becoming Bashy-coreutils implementation
credit.

There is one canonical Go applet inventory. POSIX-required is documentation
metadata, not a lean/full or certification-specific build.

