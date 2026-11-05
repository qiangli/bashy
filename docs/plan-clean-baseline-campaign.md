# Clean-baseline campaign: foundation, fences and UAT

**Foundation A status:** S561 closed engineering with no regression; its
original readiness acceptance below was superseded by the operator-approved
[closure record](s561/a4-licensed-results.md). [S571](https://github.com/dhnt/umbrella/blob/main/docs/plan-s571-posix-certification-readiness.md) owns the 67
residual decisions and full certification readiness. B/C work is not frozen.

Created at the operator's request. KISS/MVP: three outcome-based sprints, four
stories each, existing harnesses and the smallest causal fixes. All cards start
in backlog without managers; creating them does not launch execution.

## Direction

Use B -> A and C -> A/B, with composition at the product entrypoint. A owns
shell behavior and required core utilities; B owns fence/language contracts;
C owns optional providers and Yoke/product integration. Address concrete
boundary violations minimally; no wholesale architecture rewrite is planned.

This campaign replaces the legacy-reconciliation execution sequence. Sprints
539, 560 and migration 557, their patches, raw results and recovery references
remain historical references, unchanged and not falsely closed. Consult them
when a current failure matches; do not copy their backlog wholesale or require
old ticket cleanup before starting. Historical results are not current verdicts.

The operator's new campaign includes fixing confirmed A and B defects; the old
560 investigation-only restriction is not the scope of these new repair stories.
Keep separate review for broad/risky restructuring, exact licensed-run approval,
and official certification/release decisions. No release or deployment is implied.

## Sprints and stories

### Sprint 561: Foundation A: current-baseline repairs and POSIX certification readiness

Make the agreed shell and required-coreutils profile ready for formal POSIX certification using one frozen candidate and complete evidence.

| Story | ID | Deliverable |
| --- | --- | --- |
| A1 | `25dff6cbb152` | Define the minimal foundation profile and freeze its baseline |
| A2 | `78710ef53a1e` | Reproduce and classify current foundation failures |
| A3 | `3d4d3d262bdd` | Fix confirmed foundation and certification-harness defects |
| A4 | `12861d60bc4f` | Verify one certification-ready foundation candidate |

Acceptance: Required profile and test inventory fixed; current-baseline product and harness defects repaired without weakened assertions; every required result accounted for, with no unexplained failures, unresolved applicability or pending manual dispositions. Candidate hashes, commands and evidence reviewed. Readiness is not official certification or release authorization.

### Sprint 562: Fence corpora: complete required coverage and repair current failures

Make every promised fence and language corpus pass its declared platform/provider matrix with reproducible prerequisites.

| Story | ID | Deliverable |
| --- | --- | --- |
| B1 | `feae8aa96a9a` | Inventory promised fences and define the MVP corpus matrix |
| B2 | `ca8cc67e0287` | Establish reproducible fence corpus baselines |
| B3 | `6a4f4591bc8e` | Repair current fence corpus defects and prerequisites |
| B4 | `af5a35713e47` | Verify complete fence coverage on a frozen candidate |

Acceptance: Finite fence/provider/platform inventory; all required corpus cases execute and pass with reconciled denominators and no unexplained skips or unavailable coverage. Defects fixed at their owning layer, affected cases rerun, and final candidate identities recorded. Optional providers remain optional for A/B; no generic framework rewrite.

### Sprint 563: Product UAT: clean-install user journeys and defect closure

Make the agreed real-user journeys work from a clean Bashy installation, fixing every in-scope issue encountered and verifying the affected journeys again.

| Story | ID | Deliverable |
| --- | --- | --- |
| C1 | `883c9048b024` | Define the minimal end-to-end UAT journey matrix |
| C2 | `86c744779ff1` | Run early UAT smoke tests and retain actionable failures |
| C3 | `f04323bd2836` | Fix UAT defects and verify each affected journey |
| C4 | `00480e8f2c3c` | Run complete UAT against the integrated candidate |

Acceptance: Finite UAT journey/platform matrix executed from clean isolated installs; all in-scope encountered defects fixed at their owning layer and affected journeys rerun; complete final UAT passes on exact candidate identities. Includes sandbox/guard negative behavior and dependency provisioning. No production deployment, shared-install replacement or release publication is implied.

## Sequence and defect ownership

Prioritize 561/A. Begin B's inventory/baseline and C's early smoke coverage
alongside foundation work when resources permit. Within each sprint, scope ->
baseline -> incremental repair -> final verification. A fix can begin as soon as
its causal baseline exists; it need not wait for unrelated suites to complete.
Final UAT consumes stable A/B candidates. A C-provider fix needed by B's corpus
can land early; do not make all B work wait for all C acceptance.

Each first story fixes the finite inventory of required behavior, platform
coverage, prerequisites and existing commands. “All” means that declared
inventory. Record expected/executed denominators and account for every cell.
Missing prerequisites and manual/applicability decisions remain explicit blockers.

Route shell/coreutils causes to A3, fence/language causes to B3, and product or
optional-provider causes to C3. Keep the originating failed test linked and open
until the consumed repair passes. Group findings within these repair stories;
create another story only for genuinely independent ownership or acceptance.
Any failure in final verification returns to its repair story and requires an
affected rerun. No arbitrary skip, weakened oracle, or timeout increase to green.

Use one recorded current baseline and exact candidate/module/toolchain/provider
identities. Preserve the old failing control and show focused red/green evidence.
Use supported containment and finite budgets, claim shared resources before use,
and retain raw logs and hashes. Run broader required gates on a frozen candidate
rather than repeating them after every small edit. Sprint goal links require
recorded evidence, and the designated manager independently accepts delivery.
Commits carry the owning Sprint, Story and Story-ID trailers.

## Reference material

- [Previous 560/539 review](plan-s560-s539-master-execution.md) — historical
  issue map and evidence pointers; its execution order is superseded here.
- `bashy sprint show 539` and `bashy sprint show 560` — original cards and stories.
- Cloudbox `docs/migration/s560-plan.md` — previous grouped investigation scope.
- Cloudbox `docs/migration/s557/CURRENT.md` — completed reference-only migration.
- Existing private/licensed receipts remain in their recorded stores; do not
  copy raw restricted evidence into public documentation.

