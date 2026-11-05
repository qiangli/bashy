# Sprint 561 master execution plan: Foundation A certification readiness

Review date: 2026-11-03. Planning only: no sprint takeover, test dispatch,
licensed run, product change or release action was performed. Develops the
four existing stories under `plan-clean-baseline-campaign.md`; creates no new
stories and does not change their acceptance.

## Main goal

One frozen Bashy candidate whose agreed **Classic** profile (the GNU Bash 5.3 /
POSIX shell plus the required pure-Go coreutils) is **ready to hand to formal
POSIX certification**: a finite required inventory, every required result
executed and acceptable, no unexplained failure, no open applicability or
manual disposition, and reviewed hashes, commands and evidence.

Readiness is the deliverable. Official certification, an RC/v1.0.0 tag,
promotion and shared-install replacement are explicitly out of scope.

Card state at review: backlog, unclaimed, 0/4 stories closed, no continuity
checkpoint, PRIORITY-FIRST scheduling (A1 is the only P0).

## Story chain

| Order | Story | Kind / prio | Gate to start | Exit evidence |
| --- | --- | --- | --- | --- |
| 1 | A1 `25dff6cbb152` profile + frozen baseline | requirement / P0 | claim card | inventory + immutable baseline manifest |
| 2 | A2 `78710ef53a1e` reproduce + classify | test / P1 | A1 manifest | result or explicit blocker for every inventory entry |
| 3 | A3 `3d4d3d262bdd` fix confirmed defects | bug / P1 | per-finding causal baseline from A2 | red/green regressions, original cases rerun |
| 4 | A4 `12861d60bc4f` verify one candidate | test / P1 | A3 done + applicability/manual review closed | full-profile rerun, handoff pack, independent review |

A2 and A3 overlap: a repair starts as soon as its own causal baseline exists;
it does not wait for unrelated suites. A4 is strictly last.

## 0. Establish ownership (before A1)

- Resolve the designated conductor/manager; `bashy sprint take 561` under a
  real attributed identity, read the inbox. Do not borrow another identity.
- Link this plan from the card; `sprint checkpoint` after each step below.
- Claim shared resources (licensed host, `bashy-cert` VM, CI droplets) before use.

## 1. A1 — define the profile and freeze the baseline (P0)

Inventory, finite and written down (one table, required vs advisory column):

- **Shell, required:** GNU Bash 5.3 fixture gate (86 fixtures, authoritative
  lane `make test-bash-container`); the licensed 493-TP VSC shell arm
  (`make test-bash-system`, proprietary harness, Profile B).
- **Coreutils, required (already decided — not an A1 choice):** the Sprint
  341 base scope (`../vsc-pcts-harness-kit/docs/SPRINT-341-BASE-SCOPE.md`,
  routed in `docs/posix-command-coverage.md`): the 102 mandatory VSC utility
  targets via exactly 92 Go applets + 10 Bashy shell commands, **zero external
  providers**, plus `gencat` in Go; measured via Profile D (C as diagnostic).
  The 14 optional targets (`ar bg ctags ex fc fg jobs localedef make more nm
  strip talk vi`) do not gate. The product is pure Go: no utility is added,
  re-implemented or provider-backed for this sprint. Pending decisions (e.g.
  `find -exec` NO-list reversal) are recorded as decisions, not included.
- **Supported certification environment:** OS/arch, locale, PTY, containment,
  provider PATH, toolchain (`scripts/go-product.sh`, Go 1.27.2 overlay,
  `CGO_ENABLED`, build tags).
- **Advisory only:** yash, XCU/Oils/Austin/multishell differentials, zsh,
  uutils scoreboard (contained runner only). They inform, they do not gate.

Baseline manifest (immutable, hashed): bashy commit, every go.mod pin and
replace (sh, coreutils, yoke, readline…), toolchain identity, product binary
hashes (`.real` + launcher), harness commit, fixture corpus SHA-256, provider
versions, host/container image digests.

Boundary audit, minimal: record actual B→A / C→A/B violations reaching the
Classic graph (existing dependency ratchets such as
`TestGoSourceFrontEndIsNotLinkedIntoClassicBash`); fix only what blocks the
profile. No broad refactor. No legacy 539/560/557 reconciliation prerequisite.

**Exit:** inventory table + manifest committed, reviewed by the manager.

## 2. A2 — reproduce and classify (P1)

- Run each required suite once against the A1 baseline, under its supported
  containment and fixed bounds (hermetic container for Bash 5.3; licensed host
  for VSC with the exact supported approval; contained runners only for C/D).
- Classify every non-pass as **product defect / setup / applicability /
  manual review**, with raw verdict, denominator (expected vs executed) and
  artifact hash.
- Consult 560's 3059/3063 receipts only to match causes already seen (e.g. the
  53 empty-symlink setup failures, man threshold, EOF/editor cases, 9 INSPECT).
  Historical verdicts are never imported as current results.
- Hand each confirmed product/harness defect to A3 immediately; route fence
  causes to 562/B3 and provider/product causes to 563/C3, linked back here.

**Exit:** every inventory entry has a result or an explicit blocker
(cause + next action).

## 3. A3 — fix confirmed defects (P1, incremental)

- Smallest causal fix at the owning layer: `../sh` for interpreter semantics,
  `../coreutils` for utilities, `tools/bash53suite`/harness for harness bugs.
  Publish the sibling first, then move the pin (`bashy mod sync`).
- Each fix: preserved failing control, focused regression test, red→green,
  original case rerun, previously-green behavior protected (`go test ./...`,
  `make test-bash TESTS=…` while iterating).
- Applicability/manual items go through the proper authority; raw results are
  never rewritten. No skip, weakened oracle or timeout increase to get green.
- Large/risky or protected-runtime changes: present the exact diff for review
  first. Licensed reruns need their own approval.
- Commits carry `Sprint: 561`, `Story:`, `Story-ID: 3d4d3d262bdd`.

**Exit:** all confirmed in-scope defects fixed, affected original cases rerun.

## 4. A4 — verify one candidate (P1, final)

- Freeze the candidate (new manifest, same toolchain/providers as declared).
- Rerun the *whole* required profile once on it: hermetic Bash 5.3 gate,
  licensed 493-TP arm, required C/D utility set, standalone `GOWORK=off`
  build, cross-builds for the release platforms, product-wrapper build.
- Compare against prior green coverage; every denominator reconciled. Any
  failure goes back to A3 and requires an affected rerun.
- Handoff pack: identities/hashes, reproducible commands, results, residual
  advisory findings, known limits. Restricted raw evidence stays in its store.
- Independent review by the manager; then dev-host rebuild/`make install`
  smoke per the delivery gate if any Bashy change landed.

**Exit:** sprint acceptance met. No certification claim, no release.

## Decision points and stopping rules

- The required utility set is fixed by the Sprint 341 base scope; do not
  present it as an open profile choice. Stop and ask only where the inventory
  is genuinely ambiguous against that scope or the certification environment.
- Deadline hit = retained incomplete evidence + diagnosis, not an automatic
  retry, timeout growth or skip.
- Run broad gates on frozen candidates, not after every small edit.
- One finding grouping per repair story; a new story only for genuinely
  independent ownership or acceptance.

## Closure amendment authorized by the operator

The final handoff and explicit request to close Sprint 561 supersede the
original A4 requirement to finish every authority/manual disposition before
sprint closure. Engineering is accepted on the frozen candidate with no
regression. See docs/s561/a4-licensed-results.md for the exact 67 non-passes,
reused evidence, retired-host confirmation, lifted freeze, and outstanding
S355/S377 decisions. Full certification readiness remains unestablished;
there is no certification or release authorization.
