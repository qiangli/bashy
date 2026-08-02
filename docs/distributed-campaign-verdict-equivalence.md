# Distributed campaign / free-suite verdict equivalence

Status: implemented (W2, sprint 42, Phase 2). Scope: `scripts/campaign-distribute.sh`
and `scripts/test-campaign-distribute.sh` only — see "Non-overlap contract" below.

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

`scripts/test-campaign-distribute.sh` asserts this refusal for both the
`distribute` and `serial` subcommands, with a fully valid config supplied, so
the check cannot be bypassed by a "just this once" flag combination. Exit
code 77 is reserved for this refusal specifically, so a caller (or a CI gate)
can distinguish "correctly refused" from any other failure mode.

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

## The peer-worker / DKS integration seam

`campaign_dispatch_chunk()` in `scripts/campaign-distribute.sh` is the single,
documented shim function for placing a chunk on a peer-joined worker. Today
it runs the configured `RUN_CHUNK_CMD` in a local subprocess and only labels
the invocation with the assigned worker's role name — there is no real
network transport wired up here.

bashy#27 owns the DKS/kubectl host-profile scripts (`scripts/dks-profile.sh`
and friends) and is still in flight; this work does not reimplement profile
selection or touch any file `#27` owns. Once `#27` lands its profile
interface, integrating it is a one-function-body change:
`campaign_dispatch_chunk()` swaps its local-subprocess body for a call through
that interface. Nothing else in this script depends on the shape of that
interface.

## Unproven-hardware boundary

**Cross-worker distribution is UNPROVEN on real hardware.** The dispatch shim
above only demonstrates the harness's *logic* (manifest generation, evidence
checking, reduction, equivalence verification) against a local fake executor,
per the deterministic-test requirement below. No claim is made here that
running `campaign_dispatch_chunk` against a real remote peer (via ssh, mesh,
or a future DKS-backed transport) has been exercised on physical hosts. Hosts
in any future real-worker configuration should be named by **role**
("worker" backend/arch/… ), never by hostname, username, or other
environment-identifying detail — consistent with
`docs/chunked-fleet-conformance-plan.md`'s privacy rule.

There is also no cloudbox dependency anywhere in the peer-execution path: the
default dispatch is a plain local subprocess, and the documented integration
seam for a real transport is peer-to-peer (ssh/mesh) or the DKS profile from
`#27` — never a third-party managed sandbox.

## Deterministic tests, no cluster, no network

`scripts/test-campaign-distribute.sh` fakes the entire worker layer: every
"peer worker" is a role name string, and every chunk dispatch runs a local
fake executor script that reads a canonical `case_id -> outcome` map and
supports fault-injection knobs (unreachable worker, empty result, dropped
case, injected foreign case, outcome-swap). Nothing in the test touches a
real network socket or a real cluster. It must pass under both shells this
repo ships:

```sh
/bin/bash scripts/test-campaign-distribute.sh
bashy scripts/test-campaign-distribute.sh
```

Both were run for this change and both passed with no bashy-specific defect
found — no workaround was needed in the scripts.

## Non-overlap contract

This work owns exactly:

- `scripts/campaign-distribute.sh` (new)
- `scripts/test-campaign-distribute.sh` (new)
- `docs/distributed-campaign-verdict-equivalence.md` (this file, new)

It does not create, edit, or depend on the internals of anything bashy#27
owns: `scripts/dks-profile.sh`, `scripts/test-dks-profile.sh`,
`scripts/dks-author-qa-refs.sh`, `scripts/dks-native-result.sh`,
`scripts/dks-release-gate.sh`, `scripts/k8s-job-aggregate.sh`, or
`docs/plan-dks-build-test-deploy.md`. The only place this work is aware that
interface exists is the single shim function documented above.

## Licensing

VSC-PCTS suite material (if a "free suite" corpus originates there) is bound
by that program's redistribution terms. This work never copies suite content
into the repository or into these scripts — `CASES_FILE` carries test-case
**IDs** only, supplied by the caller at run time, never suite text.
