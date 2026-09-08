# Sprint 138: resource-aware sprint management

Use `bashy sprint monitor` for a combined host and model-account snapshot:

```sh
bashy sprint monitor
bashy sprint monitor 138 --json
bashy sprint monitor --watch --json --duration 30s
bashy models usage --vendor openai --json
bashy models limits --json
bashy models budget --json
```

The selected sprint retains other consumers of shared capacity. Reports include
source/freshness/unknown status; missing telemetry never becomes zero usage or
healthy capacity. Watch updates and manager notices are bounded and deduplicated.
Observation neither starts work nor acknowledges the recipient's unread mail.

Host snapshots, process attribution, inventory scans and provider refreshes are
shared across clients. New workload records include native process birth identity;
legacy PID-only records remain unverified. Native managers observe independently
of a busy model turn. Actual weave/chat and explicitly expensive scheduled work
reserve capacity before launch. Owned pause/resume and cleanup require verified
identity and lifecycle evidence; dirty, shared, active and uncertain work remains
protected. Cleanup reports apparent bytes, not a physical-space guarantee.

Policy is explicit. `models budget configure --file policy.json` validates;
`--apply` publishes it. Host sampled pressure floors and atomic reserved-demand
ceilings are separate controls. Unknown token/spend demand cannot satisfy a hard
budget. Separate machine-local authorities are not global distributed quotas.

Remote observation uses `sprint monitor --host REGISTERED_ALIAS`. Explicit
`dag capacity plan`, `dispatch`, `queued` and `reconcile` commands enforce access,
data/workspace policy, target-side capacity and checkout/runtime/executable
verification. Refused work remains queued. Remote fixtures exercise actual
transport through a permitted local endpoint; no customer data, live provider
credentials or remote provisioning were used for acceptance.

Operational limits:

- Live model adapters require explicitly configured OpenAI organization usage/cost
  access or an operator-provided Claude statusline bridge. Every other configured
  provider stays visible with truthful unsupported/unknown status. External
  harness usage can remain estimated or unknown; fixture success is not evidence
  of a customer's account access or exact spend.
- Windows native host/admission/executable tests run in CI. Guarded remote
  execution remains explicitly queued on unsupported platforms. Linux/macOS
  placement still requires fresh headroom meeting policy; estimated metrics
  cannot silently authorize a hard constraint.
- Declared executable hashes do not attest descendants, libraries or a sandbox.
  Uncertain external descendants retain capacity until explicit verified
  reconciliation. Do not delete durable receipts as a quota reset.

## Recorded verification

The product candidate was coreutils `c14c958b` and Bashy `ac08764`; this completion
commit changes only this document and story metadata. Final publication and
installed-image hashes are recorded in the umbrella's Sprint 138 evidence.

- P0 large-history CPU fell 93.1% in the retained comparison; unchanged history
  no longer causes repeated decoding. Installed watcher evidence separately
  preserves unread mail and lease release. No billed token savings are claimed.
- Final component race gates exercised actual observation, reservation, lifecycle,
  remote-policy and monitor regressions. Core ratchet: 7,229 passed, zero failed.
  Cross-platform vet, Bashy Go tests and all 86 shell fixtures passed.
- All three core platform jobs and all five Bashy jobs passed on the product
  candidate. Core Windows performed actual native resource, multiprocess budget
  and executable checks; compilation alone was not accepted.
- Eight real monitor loops, ten active sprints, two fixture providers and
  200,000 events/56 MB measured 1.0174% aggregate CPU and 61.484 MiB additional
  RSS over at least 60 seconds. Host refreshes were shared (14); each provider
  was called twice. Output stayed below 4 KiB per record; no inference occurred.
- The manager revised the original proposed 1% CPU target to an explicit 1.5%
  release ceiling after five retained failed measurements and profiling. The
  original target remains unmet; memory, freshness, polling and output limits
  were unchanged. Historical failures were not rewritten.
- Installed CLI checks verified cold/cached snapshots, provider filters, a
  duration-bounded watch and unchanged authority files. The 60-second installed
  watcher proof verified its actual executable, unread preservation and shutdown.

Runnable policy examples and deterministic demonstrations are in coreutils
`pkg/llmbudget/README.md`, `pkg/weave/resource_lifecycle.md` and
`pkg/dag/capacity.md`. The umbrella's `docs/sprint-138-master-execution-plan.md`
records scope, reconciliation, delegation, measured tradeoffs and final closure.
