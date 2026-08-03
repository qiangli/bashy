# Utilization-aware auto placement for the campaign k8s transport

## Problem

`scripts/dks-select-node.sh` already scores nodes by `max(live metrics, reserved
Pod requests)`, penalizes Dragon / control-plane, prefers novicortex / novidesign,
and understands physical-host metrics for virtual nodes. But
`scripts/campaign-distribute-k8s.sh` still **required** a static `WORKER_NODES`
role=node map and hard-pinned every role — so campaign / free-suite shards
bypassed the resource-aware selector entirely.

## Seam added (transport only; the selector is untouched)

The single missing seam was `campaign-distribute-k8s.sh`'s role→node resolution.

- **Manual mode is unchanged.** When `WORKER_NODES` is set it is a fully
  backward-compatible override: every role is hard-pinned and the selector is
  never consulted. With no shard sizing the Job manifest is byte-for-byte the
  same as before (no `resources` block).
- **Auto mode is opt-in** (`CAMPAIGN_AUTO_PLACEMENT=1`, only when `WORKER_NODES`
  is unset). At `preflight` each logical `WORKERS` role is resolved to a
  **distinct** eligible node with `dks-select-node.sh`. Each resolution feeds the
  nodes already chosen through `DKS_SELECT_EXCLUDE_NODES`, so one point-in-time
  preflight cannot select a node twice (the selector's documented concurrency
  limitation, handled exactly as it prescribes).
- **Resolved once, read many.** `preflight` persists the `role=node` map to a
  run-local, non-secret file (`CAMPAIGN_K8S_PLACEMENT_FILE`, set by
  `campaign-distribute.sh` to `$OUT_DIR/placement.env`), written
  write-then-rename so a reader never sees a partial map. `dispatch-chunk` reads
  that file and **never** re-runs the selector — the placement is decided exactly
  once and cannot drift or be recomputed concurrently.
- **Requests are enforced.** Auto placement accepts configurable
  `CAMPAIGN_SHARD_OS` / `_BACKEND` / `_ARCH` and per-shard
  `CAMPAIGN_SHARD_CPU` / `_MEM` (defaulting to `linux` / `vk-podman` / any and,
  in auto mode, `1` / `1Gi`). The value that **sizes selection** is the same
  value stamped into the Job's `resources.requests`, so Kubernetes enforces the
  reservation and the next selection sees this shard as used capacity.
- **Fail-closed.** If the selector finds no eligible node for any role
  (insufficient / stale / unavailable capacity), preflight refuses (exit via
  `resolve_auto` → 15) before any Job is created.

## Preserved invariants

Peer-only CA identity gate, distinct-physical-worker validation (node name **and**
host+backend identity tuple), evidence sidecars, `MODE=cert` refusal, cleanup
ledger, and fully hermetic fake-kubectl testing all remain — the auto seam runs
*before* the identity gate's node lookups and reuses the same validation path.

## Tests (`scripts/test-campaign-distribute.sh`, GNU bash + Bashy)

30. Auto happy path: busy Dragon loses to idle novicortex/novidesign; two roles
    resolve to two distinct preferred nodes; the persisted map is written; the
    emitted Jobs carry `resources.requests` matching the selection request.
31. A busy novicortex drops out (insufficient headroom) → novidesign selected.
32. Two roles never collide on one node (exclusion feedback).
33. Insufficient capacity (impossible CPU request) fails closed, no Job created.
34. Required-but-absent metrics fails closed, no Job created.
35. Manual `WORKER_NODES` overrides even with `CAMPAIGN_AUTO_PLACEMENT=1`; the
    selector is not consulted and the manifest gains no `resources` block.

Selection scoring itself keeps its own suite (`scripts/test-dks-select-node.sh`);
this change does not weaken it.
