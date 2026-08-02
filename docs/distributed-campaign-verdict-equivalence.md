# Distributed campaign / free-suite verdict equivalence

Status: implemented. W2 (sprint 42, Phase 2) delivered the
reduction/equivalence harness; W2b delivered the real peer-DKS transport it
was missing. Scope: `scripts/campaign-distribute.sh`,
`scripts/campaign-distribute-k8s.sh` and `scripts/test-campaign-distribute.sh`
— see "Non-overlap contract" below.

## Why this exists

Fanning a conformance/utility suite across peer-joined workers is only worth
anything if the distributed verdict is **identical** to the serial verdict —
otherwise distribution has silently changed what "pass" means. This is the
same equality property `docs/plan-distributed-chunked-execution.md` names as
the permanent regression test for any chunked runner ("Chunked ≡ serial, case
for case... diffed from per-case records, not from summary counts"). This doc
and its scripts are the free-suite/campaign-arm instance of that property.

## THE CERTIFICATION LINE

This is the hardest constraint here and it is non-negotiable:

- **Certification runs on ONE unchunked, exclusive system-under-test.** A
  cert run is never chunked, split, sharded, parallelised, or distributed.
- **Only the CAMPAIGN arm and free suites may be distributed.** The
  authoritative bash-5.3 gate (`make test-bash`, serial, single-host, 86/86)
  and any VSC-PCTS certification run are untouched by anything in this file.
- **Distributed results here are DEVELOPMENT EVIDENCE ONLY.** They may never
  be presented as, or promoted into, a certification result.

This is enforced structurally, not just documented: `scripts/campaign-distribute.sh`
checks `MODE` as the very first thing it does, before parsing any other
argument or touching any file:

```sh
MODE="${MODE:-campaign}"
if [ "$MODE" = cert ]; then
  echo "campaign-distribute: REFUSED — MODE=cert may never distribute or chunk a run." >&2
  exit 77
fi
```

`scripts/campaign-distribute-k8s.sh` carries the identical check as defense
in depth, so even a caller that somehow reached the transport directly is
refused. `scripts/test-campaign-distribute.sh` asserts this refusal for the
`distribute` and `serial` subcommands, for the transport script itself, and
with the fake seams and `CAMPAIGN_TRANSPORT=k8s` supplied, so the check
cannot be bypassed by a "just this once" flag combination. Exit code 77 is
reserved for this refusal specifically, so a caller (or a CI gate) can
distinguish "correctly refused" from any other failure mode.

If a cert run is ever needed, it must go through the existing, unchunked
harness (`make test-bash`, `bashy dag suites.md`'s non-chunked targets) —
never through this script, in any mode.

## What "verdict equivalence" means

Equal totals are not equal verdicts. A distributed run that loses 3 tests and
gains 3 unrelated passes has the same pass/fail count as the serial run and
is **wrong**. Equivalence means the per-test outcome **set** — `{test_id:
outcome}` for every test_id in the corpus — is identical between the serial
run and the distributed run.

`scripts/campaign-distribute.sh verify` runs both:

1. `serial` — the whole corpus dispatched as one chunk to a single worker
   role (`serial-exclusive`). This is a *development baseline* for the
   equivalence check, not a certification run — the real certification
   baseline is the existing harness (see previous section).
2. `distribute` — the corpus fanned out across the configured chunk/worker
   manifest.

It then diffs the two sorted `test_id outcome` files directly (not the
summary JSON) and reports `CAMPAIGN_VERDICT_EQUIVALENT` only if they are
byte-identical; otherwise `CAMPAIGN_VERDICT_MISMATCH` with the actual
disagreeing lines. `scripts/test-campaign-distribute.sh` includes an
adversarial case built specifically to defeat a count-only check: a fake
worker swaps two tests' outcomes only on the distributed path, producing the
*same* pass/fail/skip totals as the serial run while disagreeing on which
test passed. The harness must — and does — reject this as a mismatch.

## Evidence invariants

A missing chunk result, an unreachable worker, a lost log, or an empty result
file is a **failure of the reduction**, never an implicit pass or an implicit
skip. "No failures reported" from a worker that never ran is exactly the
false-green this harness exists to prevent (see `docs/absence-of-evidence.md`
for the broader pattern this repo has hit before). Concretely,
`campaign-distribute.sh distribute` fails closed on:

- a dispatch command that exits non-zero (worker unreachable / executor
  error);
- an empty result file for a chunk that was assigned one or more cases;
- a chunk reporting a case set that does not exactly equal its assigned set
  (a dropped case, or a foreign/duplicate case smuggled in from elsewhere);
- a manifest that assigns one case to two different chunks (each chunk can
  faithfully report its own assigned set and the run must still fail, because
  the *corpus-wide* coverage check independently requires every case to be
  reported by exactly one chunk);
- a manifest that omits a case from every chunk (no chunk can produce
  evidence for a case it was never told to run).

Every expected chunk (`0..CHUNKS-1`) is positively accounted for — the
harness does not infer "no news is good news" from a chunk it never heard
back from.

## Determinism and replay

Chunk assignment (`test_id -> chunk_id`) is derived from a stable CRC
(`cksum` of `"$SEED:$test_id"`, mod `CHUNKS`) — no `$RANDOM`, no dependence on
fleet capacity or arrival order. The same `(SEED, CASES_FILE, CHUNKS)` always
produces the same manifest, on any host. `MANIFEST=<path>` lets a caller pin
that manifest to a file; pointing a later run at the exact same file replays
the exact same chunk assignment, which is what makes a disagreement
reproducible rather than a one-off flake.

## The real transport: per-chunk Kubernetes Jobs on the peer cluster

`CAMPAIGN_TRANSPORT=k8s` is the **only real transport**, implemented in
`scripts/campaign-distribute-k8s.sh`. With no transport selected,
`distribute` refuses (exit 8) — there is no default, and the local-subprocess
dispatch that W2 originally shipped is no longer reachable as an execution
mode (see "Transports and verdict markers" below). Per chunk, the transport:

1. **Creates one Kubernetes Job** on the peer cluster, `backoffLimit: 0`,
   `restartPolicy: Never`, with a `ttlSecondsAfterFinished` backstop.
2. **Pins by node name** (`spec.nodeName` — no scheduler, no label matching)
   to the node the chunk's worker role maps to in `WORKER_NODES`
   (`role=node` pairs; hosts are named by role, never hostname/user).
3. **Awaits completion** with a hard timeout: a Job that never scheduled,
   wedged, or failed is a failed chunk — never a pass.
4. **Collects logs as attributed evidence**: the pod's first line must be a
   `CAMPAIGN_CHUNK_EVIDENCE` header stamped *inside the pod* with the Job
   name, chunk id, worker role, suite, and the node name the kubelet reported
   (`spec.nodeName` via the downward API). Collection cross-checks the pod's
   `ownerReferences` uid against the Job uid (a pod not owned by this chunk's
   Job is foreign evidence, rejected), the observed `.spec.nodeName` against
   the pin (a pod that ran elsewhere means the pin did not hold, rejected),
   and the header against both. An evidence sidecar
   (`bashy.campaign.evidence/v1`: suite, chunk, worker, job, job uid, pod,
   node, log path) is written per chunk — an unattributed result is not
   evidence, and the reduction refuses a chunk that lacks one.
5. **Deletes the Job** after collection. Cleanup is three-layered: each
   dispatch deletes its own Job on every exit path (including INT/TERM), the
   parent sweeps a `jobs.created` ledger from an EXIT trap (Jobs are recorded
   in the ledger *before* `kubectl apply`), and the TTL is the final
   backstop. No workload is leaked into the cluster, including on failure and
   interrupt.

After all chunks, the reduction additionally asserts the **observed**
(API-reported, not requested) placement covered at least two distinct nodes —
two chunks landing on one worker is not distribution.

### Worker distinctness and the dhnt#4 host-label trap

`preflight` runs before any Job is created and refuses (non-zero, loudly) if
the pinning is not real distribution: fewer than two distinct nodes, two
roles pinning one node, or a pinned node that does not exist in the peer
cluster. The trap found in the dhnt#4 gate is handled explicitly:
`outpost.dhnt.io/host` labels a **host**, not a node — a host running two
virtual backends presents two nodes with the identical host label. Preflight
therefore resolves each pinned node's `(outpost.dhnt.io/host,
outpost.dhnt.io/backend)` identity tuple and refuses when two distinct node
names collapse to one worker identity; the same host with distinct backend
discriminators is accepted as two workers.

### The D3 cluster-selection shim

Cluster selection is bashy#27/D3's job, consumed through **one** function —
`campaign_k8s_resolve_kubectl()` in `scripts/campaign-distribute-k8s.sh` —
which sources `scripts/dks-profile.sh` and calls `dks_resolve_kubectl`. If
the profile interface changes, that one function body is the entire
integration. The transport is peer-only: `DKS_PROFILE=peer` is forced, an
explicit cloudbox profile is refused (exit 8 — no cloudbox dependency in the
peer path), and an ambient `$KUBECTL` is deliberately **not** honored — the
profile is authoritative over the environment, so an inherited cloudbox
kubectl can never silently steal a peer run. A missing peer kubeconfig fails
loudly through the profile (exit 9), never falls back.

## Transports and verdict markers

Only a real peer-transport run may emit the promotable `CAMPAIGN_VERDICT`
marker. The fake paths exist **only** as unit-injection seams for the
deterministic gate and are structurally non-promotable:

| Path | Selected by | Verdict marker | Evidence class |
|---|---|---|---|
| k8s, real kubectl via D3 peer profile | `CAMPAIGN_TRANSPORT=k8s` | `CAMPAIGN_VERDICT` | `development-only` |
| k8s, fake kubectl | `CAMPAIGN_K8S_FAKE_KUBECTL=<path>` | `CAMPAIGN_FAKE_TRANSPORT_VERDICT` | `logic-check-only` |
| local subprocess (the W2 dispatch) | `CAMPAIGN_TEST_FAKE_TRANSPORT=1` | `CAMPAIGN_FAKE_TRANSPORT_VERDICT` | `logic-check-only` |
| none | — | refused, exit 8 | — |

`verify` mirrors the split: a fake-transport equivalence reports
`CAMPAIGN_LOGIC_EQUIVALENT` (a claim about the reduction logic), never
`CAMPAIGN_VERDICT_EQUIVALENT` (a claim that distributed execution matched
serial). The gate asserts a fake run can produce neither promotable marker.
And even the real thing is **development evidence only**: note that
`"evidence"` never says anything stronger than `development-only` — there is
no marker, mode, or flag combination under which this harness's output is a
certification result.

## Unproven-hardware boundary

**Live cross-worker execution is UNPROVEN — no two-machine run has
happened.** The k8s transport above is real code exercised end-to-end
(preflight, Job manifest, placement/identity evidence, collection, cleanup)
but only through the fake-kubectl injection seam that simulates a small peer
cluster on disk. No claim is made that a chunk Job has run on physical
peer-joined hardware; the first real run must be labelled as such and its
results treated as development evidence only, like everything else here.
Hosts in any real-worker configuration are named by **role**
(`worker-a=peer-node-1 …`), never by hostname, username, or other
environment-identifying detail — consistent with
`docs/chunked-fleet-conformance-plan.md`'s privacy rule.

## Deterministic tests, no cluster, no network

`scripts/test-campaign-distribute.sh` stays cluster-free and network-free by
construction, through two injection seams:

- **`CAMPAIGN_TEST_FAKE_TRANSPORT=1`** exercises the reduction/equivalence
  logic against a local fake executor that reads a canonical
  `case_id -> outcome` map and supports fault-injection knobs (unreachable
  worker, empty result, dropped case, injected foreign case, outcome-swap).
- **`CAMPAIGN_K8S_FAKE_KUBECTL=<path>`** exercises the k8s transport
  end-to-end against a fake kubectl that simulates a small peer cluster on
  disk (node objects with outpost host/backend labels — including the dhnt#4
  twin-node trap — applied Job manifests, a deletion log, and logs derived
  from the recorded manifest), with its own fault knobs (wedged Job, empty
  logs, mis-scheduled pod, foreign pod owner).

Neither seam is reachable as a real execution mode, and the gate asserts both
force the non-promotable marker. It must pass under both shells this repo
ships:

```sh
/bin/bash scripts/test-campaign-distribute.sh
bashy scripts/test-campaign-distribute.sh
```

Both were run for this change and both passed with no bashy-specific defect
found — no workaround was needed in the scripts.

## Non-overlap contract

This work owns exactly:

- `scripts/campaign-distribute.sh` (W2, extended here)
- `scripts/campaign-distribute-k8s.sh` (new — the real peer-DKS transport)
- `scripts/test-campaign-distribute.sh` (W2, extended here)
- `docs/distributed-campaign-verdict-equivalence.md` (this file)

It does not create, edit, or depend on the internals of anything bashy#27/D3
owns: `scripts/dks-profile.sh`, `scripts/test-dks-profile.sh`,
`scripts/dks-author-qa-refs.sh`, `scripts/dks-native-result.sh`,
`scripts/dks-release-gate.sh`, `scripts/k8s-job-aggregate.sh`, or
`docs/plan-dks-build-test-deploy.md`. The only place this work consumes that
interface is the single `campaign_k8s_resolve_kubectl()` shim documented
above.

## Licensing

VSC-PCTS suite material (if a "free suite" corpus originates there) is bound
by that program's redistribution terms. This work never copies suite content
into the repository or into these scripts — `CASES_FILE` carries test-case
**IDs** only, supplied by the caller at run time, never suite text.
