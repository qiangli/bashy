# Sprint 560 and selected Sprint 539 follow-ups: master execution plan

Review date: 2026-11-03, America/Los_Angeles. Planning only; no sprint takeover,
implementation, test dispatch, release, or host-install change performed.
This plan develops existing stories; it creates no additional implementation
stories and does not supersede their acceptance or operator restrictions.

## Outcome and current evidence

Deliver bounded, verified repairs on the published reference-only migration,
preserving protected conformance behavior and an honest record of unresolved
failures. Pair Sprint 560's six causal groups with selected Sprint 539 fixes,
keeping each change owned by its original story.

Both cards are backlog and unclaimed: 560 has 0/6 stories closed; 539 has 0/9.
Neither has a continuity checkpoint. Sprint 560's historical Bashy1/2/3,
Ycode1 and Yoke1 references are preserved review material, not live assignments
or accepted repairs. Sprint 557 is complete; older migration status documents
must be read through Cloudbox's `docs/migration/s557/CURRENT.md`.

The original control is frozen dhnt `3e7ffaafd87a833663df198164a8d83597bdfd0b`;
the published migration roots are public `d99602557b7a3b60c38e0b1d03068fffce39f8c2`
and private `d5cd16577bea59d732786ce184e3b7f7b2029a38`. Capture actual component
heads, pins and product hashes before changing anything; these references are
controls, not an instruction to reset a checkout.

The retained full POSIX D result contains 9,337 execution purposes, including
4 FAIL, 54 UNRESOLVED and 9 INSPECT. All verdicts match the October 6 archive;
different product/toolchain/host identities prevent an identical-build or
identical-cause claim. Shell-only 493 has zero blockers. Neither result establishes
official certification. The Windows printf crossover already establishes a
pre-existing symptom; more repetitions merely to establish occurrence are unnecessary.

## Selection from 539

| Story | Proposed treatment | Relationship to 560 |
| --- | --- | --- |
| F8 / 2855 / `2ff43d03b869` | First repair priority: explicit sandbox launch | Inform 3064's provider boundary; implementation remains F8 |
| F6 / 2853 / `5585a1e08aff` | Child guard inheritance and external-effect enforcement | Design alongside F8; preserve separate security acceptance |
| F1 / 2848 / `73f5b066f684` | Native git short flags plus show/archive support audit | Standalone bounded delivery; no duplicate 3060 ownership |
| F2 / 2849 / `2c49fae01ec9` and F3 / 2850 / `87c22f17ad9f` | One fixture investigation, separately evidenced stories | Coordinate with 3063 environment work |
| F0 / 2847 / `fb86f77012fd` | Start ledger early; update throughout delivery | Records release-specific facts, not blanket 560 waivers |
| F4 / 2851 / `33a23e6a413a` | Small generated-document repair | Include when measured catalog/image inputs are available |
| F5 / 2852 / `7fbc200115f2` | Diagnose durations, then propose a bounded gate split | Coordinate with 3063; do not silently increase deadlines |
| F7 / 2854 / `94ac7c89208a` | Verify historical completion and reconcile records | No new promotion or shared-install downgrade |

F7's October 31 card note reports rc.23 promoted on October 30 at 09:57:23Z.
That is retained evidence, not a fresh release check. Before acceptance, retrieve
the original candidate/promoted digest comparison, spot check, announcement and
installation receipts. A promotion timestamp alone does not satisfy every F7
criterion. F0 was explicitly reported incomplete. Known issue 6 belongs in the
ledger too, as documented in F8; prior rc.23 approval is not a future RC waiver.

## Execution sequence

### 0. Establish identity, ownership and the evidence manifest

Before execution, resolve the canonical manager for each card, read its inbox,
and use supported sprint lifecycle commands. Do not guess a manager or take an
existing identity. The current review's inbox attempt was refused because this
session has no attributed Bashy agent identity; no inbox contents were read.

Reconcile 539's old `poc/dhnt/bashy` story locations with the current checkout by
story UUID. Its JSON graph has goal links, while the plain card still reports
stories uncovered; inspect resolution before recreating links. Link 560's six
groups into the execution goals and reference this plan from the card when the
manager is established. Preserve original story IDs and historical evidence.

Record dependency-driven ordering: F7 is historical reconciliation, F8 is the
live P0 repair. If prerequisite P1/P2 work must precede a runnable P0, record the
override rather than relying on prose here to change the scheduler.

For each causal subitem record: owning story, original command, source/module
tuple, toolchain and CGO/build tags, fixture/oracle, OS/environment, denominator,
deadline, raw artifact/hash, classification, patch provenance, and validation.
Classify setup, product, incomplete coverage, or unresolved cause separately.
Identify already-delivered Stage C hunks before applying anything again.

### 1. Repair the explicit sandbox and settle the child-execution contract

F8: reproduce doubled argv0 against the image ENTRYPOINT; bind the host launcher
to the running candidate instead of PATH; diagnose native Windows exit 126
independently. Review image/entrypoint variants before selecting argv behavior.
The inspected `yoke/pkg/shape/contain/oci.go` still defaults an empty Bin to
`bashy` and appends argv after the image, consistent with the story's mechanism.

Prove exact payload output on WSL and native Windows using a fresh isolated
BASHY_HOME and authenticated candidate/image identity. Include failure cases:
missing image/runtime and launch failure remain closed; no unsandboxed fallback;
network and resource restrictions remain effective. Linux coverage supports the
shared launcher behavior but does not substitute for native Windows.

F6: Bashy child shells inherit the guard and reject malformed boundary state;
undeclared external programs are refused, including atlas-unknown tools. A
declared and granted external effect grants that program's effects; arbitrary
external containment requires explicit sandbox selection. No automatic sandbox
is introduced. Test child Bashy net denial, undeclared external refusal, and
allowed effects on Linux and native Windows. F8 enables the sandbox branch of
integration acceptance, not every F6 implementation step. If the minimal fix
requires protected Sh changes, present the exact diff and decision first.

### 2. Land small metadata and fixture repairs

3060 / `8738ca8e9540`: reconcile Yoke source metadata with the dependency actually
pinned in Bashy's go.mod, without uplifting the dependency. The inspected
`srcModulePins` still contains Yoke `3fda1c1...`; the frozen evidence records the
linked `89df...` mismatch. Validate source lookup and standalone module resolution.
Review Bonsai vet, Gotreesitter paths and Yoke platform failures as separate
causes within this same story; do not replay mixed historical commits.

3061 / `12e13646ce7e`: check whether the two Stage C fixes are already integrated.
If missing, extract only registration hunks from `138c566`/`623524d`, retaining
isolated XDG/config paths and real HOME with explicit `register --out`. Independently
add the unchanged gspt directory before Docker dependency download if still needed.
Preserve compiler, pins, assertions and build recipe. Validate original portal
and Podman lanes at 25/30-minute bounds, then single/split compose at 45-minute
bounds. These are existing job ceilings, not estimates of total delivery time.

F1: audit actual command callers before implementing flag mappings; verify
push/fetch/clone/commit/log/show/archive semantics with local temporary remotes,
including flags that alter behavior. Run without host git on Linux and Windows.
If the owning implementation is protected Coreutils, isolate the proposed change
and obtain the separate decision required by 560 rather than assuming F1 overrides it.

F2/F3: the current fixture still invokes `shPath(t)` and shell `[`; establish the
resolved interpreter and remove ambient shell/toolchain-fetch dependencies while
preserving the malformed-engine rejection assertions. Test network-disabled,
isolated configuration on Windows and Linux. Do not make product fallback more
permissive to fix the test. F4 regenerates the table from measured image/catalog
inputs and verifies the relevant workflow; historical counts are not current proof.

### 3. Diagnose incomplete environments and private failures with fixed bounds

3063 / `d6b1b2029365`: address Alpine ELF-loader mismatch, missing node/pnpm,
Gotreesitter parity inputs, named-pipe prerequisites and authenticated corpus/Tour
artifacts. Preserve original failed runs and label corrected-environment results.
For Uutils, review the precise executable-work-mount change inside the existing
disposable, non-root, capped runner before running it; preserve no-network,
mount restrictions, quarantine and denominator validation. Never run it natively.

F5: collect package/test durations and distinguish slow execution, deadlock and
preparation. Prefer coverage-preserving shards with a reconciled test manifest.
Its story permits considering a larger Windows budget, but 3063 forbids blind
increases: any proposal must show measurements, bounded cost and retained coverage.

3062 / `61d826f2e2be`: investigate wake HTTP429 under cap4, exact Secrets CLI
contract, and browser seed HTTP503 separately. Capture the actual failing seed
endpoint and sanitized durable logs before attributing cause. Preserve the
non-short concurrency test. Keep Cloudbox9 recovery refs/bundle and guarded
workspace until a supported audited recovery proves teardown safe.

DKS zero nodes, Helm readiness and broader network/controller changes are a
decision point under 3061, not permission to replay wake/RBAC/Traefik/firewall
bundles. Present a causal repro, smallest patch, original acceptance lane and
measured resource cost before larger work or a deferral decision.

### 4. Complete protected investigations and architecture review

3059 / `1c99c75486c6`: use existing receipts first. Obtain configuration/applicability
disposition for 53 empty-symlink setup failures and the man threshold failure;
review retained EOF/editor evidence for four more failures before proposing a
bounded unchanged-PTY/locale diagnostic; document nine manual dispositions.
Also retain and classify full-Sh, glob, cancellation-race and oracle findings.
Do not rewrite raw verdicts or equate baseline occurrence with resolution.
New licensed execution requires its exact supported approval; no full D rerun
merely to repeat the already-verified historical comparison.

3064 / `2860bcfb6ffe`: audit now, implement only after direction is confirmed.
Proposed dependency arrows are B -> A and C -> A/B; A imports neither B nor C,
and B does not import C. The product assembly entrypoint may wire selected
providers. The story explicitly records conflicting operator wording about
A/B referencing C; that remains an unresolved design decision.

Produce a package/profile map including indirect imports, build tags, generated
code, initialization/registration and runtime provisioning. Distinguish required
core utilities from optional utilities, and language island contracts from OCI,
network and agent providers. The existing Classic dependency test explicitly
records go/types inside the shared interpreter; absence of the Go front-end
package does not prove complete A/B separation. Report such boundaries honestly.

Propose the smallest enforcement using existing dependency/build checks. Validate
a Bashy-only clean checkout with GOWORK=off, no private sources or mandatory
sibling checkouts, and the agreed A and A+B profiles without optional C. Coordinate
source-pin ownership with 3060. No broad parser/interpreter/Coreutils refactor is
authorized by this plan. Architecture inventory may proceed alongside bounded
repairs; speculative restructuring is not their prerequisite.

### 5. Integrate, verify and close by evidence

For each patch, preserve the original failing control and demonstrate the
intended red/green change plus relevant negative behavior. Apply small reviewed
hunks, retaining original recovery references. If a sibling changes, publish the
reviewed producer before pinning the consumer; never substitute unrelated latest
heads for the reviewed dependency tuple.

Run focused checks per patch, then freeze one integrated candidate for affected
package suites, standalone GOWORK=off checks, OS builds and required integration
lanes. Use the product build wrapper where the startup contract applies. Perform
repository-required go tests and Bash fixture gates for delivered Bashy changes;
use the hermetic Linux Bash gate for a release verdict, native Windows for Windows
claims, and retain every known failure/skip and coverage denominator explicitly.
Run broader conformance only where the changed boundary requires it and the
applicable execution authorization exists. No all-green claim from job exit alone.

Each story submission carries commits, exact artifact identities, commands,
denominators, logs, preserved failures and remaining limitations. The designated
manager independently verifies acceptance. Partial fixes remain subitem progress;
they do not close a grouped story with unresolved acceptance. Delivery commits
carry Sprint, Story and Story-ID trailers for their owning story. F0 records
what shipped, what remains and any release-specific approved exceptions.

Only then prepare a candidate release decision. The next RC identifier must be
resolved at that time; F8's historical rc.24 target is not proof that identifier
is still available. Official v1.0.0 remains prohibited pending official
certification and explicit release approval. This review authorizes no publishing,
promotion, announcements, shared-host replacement or new production access.

## Decision points and stopping rules

- Confirm 3064's dependency direction before dependent implementation.
- Review exact protected-runtime changes, costly infrastructure work and
  proposed deferrals before proceeding, as required by the 560 card.
- A deadline reached yields retained incomplete evidence and a diagnosis;
  it does not automatically authorize retry, timeout growth or a skip.
- Claim shared test resources before use; do not displace another holder.
- Use existing story ownership. No new story per failing assertion or per
  architecture violation, and no duplicate implementation in 539 and 560.
- No agents were dispatched by this review. Execution staffing follows the
  eventual manager and resource assignments, not the historical run list.

## Review sources and limits

Read both live cards and all fifteen story bodies; reviewed Cloudbox's current
migration closure, s560 plan, public baseline summary and private small-fix
shortlist, plus F6's original `db50b5e69f43` contract. Spot-inspected Bashy's
template fixture, source pins and Classic dependency test, and Yoke OCI argv
construction. These reads establish the plan, not fresh reproduction of every
failure or verification of current external release state. Raw private/licensed
receipts remain in their existing stores; none are copied into this document.
