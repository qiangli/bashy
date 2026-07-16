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
| **Steward** | **claude** (me) | own the outcome; monitor all conductors; **prevent agent overload** — if a conductor is piling work on one agent (e.g. codex), instruct it to reassign to a less-loaded, equally-capable agent; keep the human in the loop |
| **Conductor of (a)** | **codex** | drive the conformance campaign — decompose the failing set, isolate in weave, gate on our harnesses, converge to 100%; may staff workers, subject to steward load-balancing |
| **(b) auto-fixer** | **claude, delegated to self** (fork) | the parent stays steward; a forked self builds the test-speedup/auto-fixer fabric that (a) rides |
| **(c) ecosystem** | **ycode** (reactive) | when an ecosystem change would help (a), ycode drives it |

Additional conductors may be spun up and may assign agents; **the steward watches
total load and rebalances** so no single agent (codex especially) is saturated while
equally-capable agents sit idle. Capability + freedom, not habit, decides assignment
(`bashy agents --band`, `weave fleet`).

## Discipline (unchanged, load-bearing)
- **Never shell out** from the userland; the NO-list verbs exec by argv, not a shell.
- **PCTS results are not published** without The Open Group's written consent (pending)
  — no suite tallies/assertion ids in these world-readable repos.
- **The gate is evidence, never a status label** — a fixer that exits 0 is not a pass;
  run the differential.
