# The 100% Conformance Campaign — bashy's P1 story

*Plan of record, 2026-07-16. Brand-neutral (bashy/coreutils/sh). PCTS run numbers
are NOT published here — see `docs/vsc-pcts-run-status.md`; this doc states goals,
structure, and roles, all of which are ours to state.*

## (a) The goal — absolute 100%, no excuses

bashy is a **GNU bash 5.3-compatible POSIX shell + a POSIX-compliant coreutils
userland**, and the goal is a **100% pass rate** on the conformance suites.

**The bar is absolute.** "bash 5.3 / GNU also fails that case" is **not** an accepted
excuse. bashy must not fail. Where a suite charges a behavior that certified bash or
GNU themselves fail, bashy's answer is to **pass it anyway** — this is the
philosophy's *superset ceiling* (`docs/philosophy.md`: compat is the floor, the
superset is the ceiling), applied without exception. Passing where the reference
fails means bashy diverges *upward* from bash on those edges; that is deliberate, not
a regression, and every such divergence is recorded so the drop-in contract stays
legible.

The frontier today:
- **bash 5.3 fixture suite: 100%** (86/86) — the drop-in floor, held.
- **POSIX shell** (yash `-p`, the differentials, VSC-PCTS shell scenario): at
  bash-parity / done on the shell side.
- **Coreutils / utilities: the active front.** The delta is root-caused —
  regex/text-engine depth (`pkg/bre` driving `sed`/`grep`) is the largest chunk, then
  the NO-list argv-runner lift (`find -exec`/`xargs`/`env COMMAND`/`nice`/`nohup`),
  then a per-command long tail. Drive it to zero.

Measure with **our own** harnesses (publishable): `make test-bash`, the yash
scoreboard, the clean-room differential + 10-shell panel, POSIX-mode parity. The
licensed VSC-PCTS run is the private cross-check + the certification gate.

## (b) The accelerator — auto-fixer + distributed test running

To reach (a) faster, stand up an **auto-fixer + distributed test-running fabric**
across every agent registered to run the tests: chunk the conformance suites, fan the
chunks across the fleet (`dag --fleet` / the chunked-conformance lanes), route each
failing cluster to a fixer agent, gate on our own differential, and loop until dry.
This is the (a)-campaign's force multiplier — build it, then let (a) ride it. (Builds
on the CI-failure fixer, `docs/chunked-fleet-conformance-plan.md`, and the venue
runner.)

## (c) Ecosystem leverage — make Tessaro a better place for bashy

Any change in **ycode / outpost / cloudbox** that measurably speeds up or de-risks (a)
is in scope — **do it, even if it means temporarily pausing (a)**, exactly as the last
few days of "digressions" (weave submodule hydration, the metered-agent key-grant fix,
delegate-self, the ycode fork) each unblocked the fleet that drives (a). The detour IS
the investment when it makes the mesh better for the campaign.

## Roles & the org

| role | who | charter |
|---|---|---|
| **Steward** | **claude** (me) | own the host-wide outcome; decide conductor count and workstream boundaries; monitor conductors, coordinate shared merges/policy, and keep the human in the loop; never manage a conductor's workers |
| **Conductor of (a)** | **codex** | own the conformance workstream end-to-end — decompose the failing set, choose and size its worker pool, isolate in weave, steer/fail over workers, gate on our harnesses, merge, and converge to 100% |
| **(b) auto-fixer** | **steward-owned worker** (claude self-fork) | bounded accelerator work outside conductor (a); it may build test-speedup fabric but cannot merge into (a)'s repos without an explicit handoff |
| **(c) ecosystem** | **ecosystem conductor** (ycode, when activated) | owns a separately bounded ycode/outpost/cloudbox workstream; coordinates shared-repo merges with the steward and conductor (a) |

**Seat separation is mandatory: one agent must NOT simultaneously serve as the host steward
and as the POSIX/conformance conductor of (a).** The steward's value here is that it judges
(a)'s evidence independently; an agent that appoints itself, drives the fleet, and then
reviews its own convergence has collapsed the only independent layer, and every check becomes
self-report. The steward appoints and qualifies the conductor seat; the conductor never
self-selects that seat or names its successor. This is the general role contract (see the
`steward` and `conductor` skills), stated here because this campaign is where it binds.

The steward may appoint additional conductors when workstreams are naturally independent.
Each conductor alone decides how many workers its assignment needs and which agents to use.
The steward manages conductor boundaries and shared-host contention by addressing the
conductor, never by steering or reassigning that conductor's workers. The steward may also
own or delegate separate direct work when requested or when judgment makes that the clearer
path; ownership must remain explicit and non-overlapping.

## Discipline (unchanged, load-bearing)
- **Never shell out** from the userland; the NO-list verbs exec by argv, not a shell.
- **PCTS results are published only within written consent.** Shell-utility
  results are covered by the scoped 2026-07-16 grant; utilities-sweep results
  remain withheld pending follow-up.
- **The gate is evidence, never a status label** — a fixer that exits 0 is not a pass;
  run the differential.
