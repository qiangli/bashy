# Bashy Platform Retrospective: Interim Notes

Date: 2026-07-11

Meeting id: `2026-07-11-bashy-platform-retrospective-fleet-testing-block-4c30`

Status: open. These are chair/secretary interim notes, not final minutes. The
authoritative transcript remains in the local, non-repo meeting store.

Privacy note: public artifacts should avoid private hostnames, usernames, local
paths, LAN addresses, tokens, and environment-identifying details. These notes
use role-based host labels instead of private machine names.

Reference material:

- Source request:
  [`docs/bashy-platform-retrospective-original-request.md`](bashy-platform-retrospective-original-request.md)
- Meeting brief:
  [`docs/bashy-platform-retrospective-brief.md`](bashy-platform-retrospective-brief.md)

## Roster

- `codex`
- `claude`
- `agy`
- `opencode`
- Secretary: `aider`

Note: `bashy meet show` reports the meeting is open at round 8. `opencode`
returned one empty turn and one abstention, then contributed to the sequencing
poll. `codex`, `claude`, and `agy` contributed substantive turns throughout.

## Corrections to the Framing

The panel corrected two weak interpretations of the fleet failure:

- The failure was not evidence that GNU Bash 5.3 compatibility itself is flaky.
  It was evidence that work was scheduled against assumptions that had not been
  modeled.
- The blockers are not all one class. Some are host readiness problems, while
  one is a true container-lane hermeticity defect.

The blockers now classify as:

- Host readiness:
  - Remote Windows host A: non-interactive SSH/elevation missing.
  - Remote Windows host B: no usable OCI substrate through current lean bashy.
  - Remote Linux container-capable host: registry/proxy failure during image
    pull.
- Container-lane hermeticity:
  - Host-built Bash test helper binaries leaked into a Linux container through
    mounted testdata.
  - Some tests reject root execution.
  - Required utilities such as `hexdump` were missing from the generic image.

## Consensus So Far

The panel converged on narrow fixes first, not a full platform build triggered
by this incident.

The panel also converged that the narrow fixes must be shaped as product
contracts, not one-off test harness patches. The immediate artifacts should
become the first durable surface for later fleet, pipeline, and provenance work.

Explicitly deferred from the first slice:

- Full SQLite datastore.
- Durable scheduler with leases/heartbeats/resume.
- Durable-execution replay.
- OTEL dashboards and ingestion pipeline.
- Local registry mirroring into `zot`.
- Slurm/Kubernetes/cloud adapters.
- Full pipeline DSL or typed HPC language.

## Recorded Decision

The human chair recorded the implementation-sequence decision in the meeting
ledger:

> First implementation sequence: schemas-and-both. Ship the narrow fixes
> together as one slice: a stable HostFacts-oriented doctor fleet --json
> contract, the hermetic digest-pinned bash53 container lane, and minimal
> run/result envelopes sufficient for later durable fanout. Do not build the
> full datastore, scheduler, OTEL dashboards, Slurm/Kubernetes/cloud adapters,
> or pipeline DSL in this slice.

The poll machinery parsed the fixed-choice poll as `yes`, but every participant
rationale selected `schemas-and-both`. The chair decision above is the clean
record.

## First Slice Shape

The first slice should include:

- `doctor fleet --json` or equivalent host-facts preflight.
- A stable `HostFacts` schema, frozen harder than the other early schemas.
- Hermetic digest-pinned `bash53` OCI image/lane.
- Container testdata input ownership:
  - copy testdata into a writable in-container workdir;
  - rebuild `recho`, `zecho`, and `xcase` inside the container;
  - never mount host-built helper artifacts as executable test inputs.
- Non-root execution for the Bash 5.3 test lane.
- Required userland tools in the test image, including at least `gcc`, `make`,
  `hexdump`, and required locales.
- Registry/cache/image availability preflight.
- Minimal `TaskSpec` and `bashy-run-v1`/run-result envelope sufficient to avoid
  later schema migration, but not over-designed before a successful capability
  gated fleet run.
- Capability-aware placement refusal:
  - missing container provider, registry reachability, non-interactive SSH, PTY,
    auth/elevation, or toolchain facts should fail before chunk execution.

## Long-Term Architecture Direction

The strongest architecture convergence is the three-noun model:

- `HostFacts`: what a machine can actually do.
- `TaskSpec`: what a unit of work requires.
- `RunRecord`: what happened, append-only.

Placement is a join of `TaskSpec` requirements against `HostFacts`. Adapters
consume a `TaskSpec` and emit `RunRecord`s.

The panel also accepted the need for explicit data dependencies in `TaskSpec`:

- `inputs`
- `outputs`
- paths
- artifacts
- URIs

This lets Bashy borrow the useful part of Martian/Nextflow/Snakemake without
building a separate language too early.

## Venue Model

The venue vocabulary remains useful:

- `userland`
- `workspace`
- `sandbox`
- `sphere`
- `cluster`
- `cloud`

But the current implementation should avoid making all venues equal runtime
targets immediately. Current concrete consumers exist for:

- userland
- workspace
- sandbox

The rest should remain vocabulary/adapters until real consumers justify them.

## O3 Framing

OCI is load-bearing for the sandbox venue and container-normalized testing.

OTEL should initially be treated as an export/view of `RunRecord`, not the
primary source of state.

Ollama is valuable for agentic and local triage workflows, but the panel raised
the risk of making local model routing part of the scheduler critical path too
early. The safer framing is: Ollama is a managed tool/service available to
agentic verbs, not required for core placement.

## Compatibility Boundary

The panel strongly converged on preserving Bash compatibility mechanically:

- No new construct should appear in a file that `bash` would parse.
- Metadata such as `requires`, `in`, and `out` should live in markdown DAG
  headings, `.bashy.md` sidecars, or other bashy-owned descriptors.
- Early metadata should be declarative only: no loops, conditionals, or general
  expression language.
- Control flow stays in Bash bodies, where Bash compatibility is already the
  product contract.

## Datastore Direction

The preferred direction is:

- append-only file-backed event log as source of truth;
- SQLite as a rebuildable projection;
- OTEL as export/view;
- object/blob storage later for large artifacts/logs;
- graph/KB/vector layers later as query aids, not first-slice requirements.

This preserves debuggability even when a projection is stale or a host is gone.

## Risks Raised

- Overbuilding a platform from an incident mostly caused by missing host
  readiness checks.
- Spending a long effort on Windows container-provider work when immediate
  blockers may be host provisioning, not missing bashy features.
- Designing schemas ad hoc and paying migration tax later.
- Letting host artifacts leak into container-normalized test lanes.
- Making OTEL or Ollama primary state/control dependencies too early.
- Letting a DSL grow control flow and become a poorly scoped second product.
- Poll/meeting synthesis tooling can be noisy; final minutes must be checked
  against transcript, not trusted blindly.

## Current Pending Items

The later interface and ownership rounds resolved the first-slice schema,
enum-style, image-identity, and repository-ownership questions. Remaining
pending items are no longer blockers for the first implementation slice:

- Decide when to close the meeting and promote these notes into final minutes.
- Convert the converged decisions into an implementation brief and PR checklist.
- Decide whether local `zot` registry mirroring is second slice or later.
- Decide how to express requirements in `dag.md` without overloading existing
  metadata.
- Decide how to represent paired-host execution for Windows hosts that cannot
  run OCI locally.

## Interface Round: Concrete First-Slice Shape

After the interim synthesis, the panel was asked to specify first-slice
interfaces. `codex` and `claude` gave the strongest concrete proposals, `agy`
endorsed Claude's core amendments and reinforced static metadata boundaries,
and `opencode` abstained.

### HostFacts

The proposed schema identity is:

```json
{
  "schema": "bashy.hostfacts.v1",
  "schema_version": 1
}
```

Minimum fields proposed for v1:

- `host_id`
- `observed_at`
- `ttl_seconds`
- `facts_digest`
- `bashy.version`
- `bashy.commit`
- `platform.os`
- `platform.arch`
- `platform.kernel`
- `auth.ssh_noninteractive`
- `auth.pty`
- `auth.elevation`
- `oci.available`
- `oci.provider`
- `oci.version`
- `oci.rootless`
- `oci.registry_reachable`
- `oci.image_pull`
- `oci.cached_images[]`
- `tools.bash`
- `tools.gcc`
- `tools.make`
- `tools.hexdump`
- `resources.disk_free_bytes`
- `labels[]`
- `errors[]`

Important design rule from Claude, endorsed by Agy:

- Capability values should be tri-state: `true`, `false`, or `unknown`.
- `unknown` is a placement refusal, not a permissive default.

Versioning rule:

- `bashy.hostfacts.v1` should be additive-only.
- Consumers ignore unknown fields.
- Removing, renaming, or changing the semantics of a field requires `v2`.

### TaskSpec

Minimum fields proposed:

- `schema`: `bashy.taskspec.v1`
- `schema_version`
- `task_id`
- `lane`
- `venue`
- `requires[]`
- `inputs[]`
- `outputs[]`
- `depends_on[]`
- `cwd`
- `command`
- `env_allowlist`
- `fanout.chunk_id` and `fanout.total`, or a reference to a committed chunk
  manifest.

Constraints for v1:

- `inputs` and `outputs` are static, literal workspace paths, artifacts, or URIs.
- `depends_on` is a static list of logical `task_id`s.
- No dynamic expressions.
- No general-purpose metadata language.
- No parameter sweep / `matrix` in the first slice.
- `lane` should refer to a lane definition; the lane owns image digest selection.

Open point:

- Codex proposed an `image_ref`/`image_digest` field on containerized
  `TaskSpec`.
- Claude argued image digest belongs in the lane definition, not individual
  tasks, because digest bumps should not require editing every task and because
  tasks in one fanout must not drift against each other.

Current chair interpretation: put the concrete image digest in the lane
definition, and let `RunRecord` record the digest actually used.

### RunRecord / bashy-run-v1

Minimum fields proposed:

- `schema`: `bashy.run.v1`
- `schema_version`
- `run_id`
- `parent_run_id`
- `task_id`
- `attempt`
- `host_id`
- `facts_digest`
- `status`
- `started_at`
- `ended_at`
- `exit_code`
- `preflight_failure`
- `preflight_failures[]` if multiple failures are reported
- `unsatisfied_requirement`
- `image_digest`
- `code_sha`
- `input_digest`
- `output_digest`
- `log_paths[]` or `log_ref`
- `artifacts[]` or `artifact_refs[]`

Important rule:

- A placement/preflight refusal must emit a `RunRecord` with
  `status: "preflight_failed"`.
- It must not be represented as a skip or absent run.
- This preserves observability: "scheduler refused to place this work" remains
  distinct from "nothing ran" and from "the test failed."

### Preflight Failure Classes

The panel wants typed, stable failure classes. Naming is not fully settled, but
the stable conceptual set is:

- auth failure:
  - no non-interactive SSH/elevation path
  - TTY required for elevation
  - PTY required but unavailable
- host reachability failure
- stale or unknown facts
- missing or unhealthy OCI provider
- registry unreachable
- image unavailable
- image digest mismatch
- required tool missing
- capability mismatch / unsatisfied requirement
- insufficient disk
- unsupported platform
- root forbidden for lane
- host artifact leak into container inputs
- unknown preflight failure

Claude proposed a deliberately small closed enum:

- `auth_no_tty`
- `substrate_missing`
- `registry_unreachable`
- `image_digest_mismatch`
- `capability_unsatisfied`
- `tool_missing`
- `host_unreachable`
- `unknown`

Codex proposed more explicit stable strings:

- `AUTH_NONINTERACTIVE_UNAVAILABLE`
- `ELEVATION_REQUIRES_TTY`
- `OCI_PROVIDER_MISSING`
- `OCI_UNHEALTHY`
- `REGISTRY_UNREACHABLE`
- `IMAGE_UNAVAILABLE`
- `TOOL_MISSING`
- `CAPABILITY_MISMATCH`
- `DISK_INSUFFICIENT`
- `UNSUPPORTED_PLATFORM`
- `UNKNOWN_FACT`

Chair synthesis:

- The failure classes should be a closed enum in v1.
- Each failure carries at least `host_id` and the unsatisfied requirement string.
- Substrate/auth/image failures must never be reported as Bash conformance
  failures.

### Hermetic Bash 5.3 Container Lane Acceptance Criteria

The lane is accepted only if all of the following are true:

- Image is addressed by digest, not mutable tag.
- The run uses a non-root user.
- The image includes required tools and libraries, including at least `gcc`,
  `make`, `hexdump`, and required locales.
- Testdata is copied into a writable in-container workdir.
- Testdata is not executed directly from a host bind mount.
- `recho`, `zecho`, and `xcase` are rebuilt inside the container.
- Host-built executable helper artifacts are rejected or ignored.
- A negative test proves host-built helper leakage is detected and fails loudly.
- Registry/cache/image availability is preflighted before fanout.
- The lane produces byte-identical results from at least a macOS host and a
  Linux host, proving the lane is actually container-normalized.
- The lane reaches the Bash 5.3 suite's expected 86/86 result on the Linux SUT.

Important non-goal raised by Claude:

- The container lane should not replace the authoritative serial single-host
  release gate. It is the heterogeneous fleet throughput lane; the unchunked
  serial gate remains authoritative for release confidence unless explicitly
  changed later.

### Still Deferred

The latest round reaffirmed deferring:

- SQLite projections.
- Leases.
- Resume/retry scheduler.
- OTEL ingestion.
- Registry mirroring.
- Paired Windows container routing.
- Dynamic matrix expansion.
- Rich DSL/resource semantics.
- Typed stage IO beyond static paths/artifacts/URIs.

## Recommended Next Meeting Round

The previous recommended round has now happened, followed by an ownership round.
The panel has enough agreement for final minutes or an implementation brief once
the human chair confirms the meeting should conclude.

## Ownership Round: Resolved Decisions

The focused ownership round converged on the remaining first-slice questions.

### Preflight Enum Style

Use dotted lowercase failure classes with a frozen prefix set. The current
prefixes are:

- `auth.*`
- `host.*`
- `facts.*`
- `oci.*`
- `registry.*`
- `image.*`
- `tool.*`
- `capability.*`
- `disk.*`
- `platform.*`
- `lane.*`

The prefix set is closed in v1 because routing, dashboards, and placement
policy will key off it. Suffixes may be added additively within an existing
prefix. `unknown` remains the only escape hatch; preflight must not invent new
prefixes casually.

Concrete first-slice examples:

- `auth.no_noninteractive`
- `auth.requires_tty`
- `host.unreachable`
- `facts.stale`
- `oci.missing`
- `registry.unreachable`
- `image.unavailable`
- `image.digest_mismatch`
- `tool.missing`
- `capability.unsatisfied`
- `disk.insufficient`
- `platform.unsupported`
- `lane.root_forbidden`
- `lane.host_artifact_leak`
- `unknown`

Important rule: substrate, auth, image, and host-readiness failures must never
wear a Bash conformance verdict. Conformance failures and preflight failures are
separate classes of result.

### Image Identity

The lane definition owns container image identity. `TaskSpec` names a lane; it
does not carry per-task `image_digest` values in v1.

The resolved digest must still be recorded in `RunRecord`. Lane declares,
preflight resolves, and `RunRecord` records the digest actually used. This
prevents a later lane update from silently changing the meaning of old results.

### Repository Ownership

The hermetic Bash 5.3 OCI lane should live in the bashy repository first because
it is part of bashy's conformance harness and release contract.

Reusable OCI lane plumbing can move to coreutils later, after a second consumer
exists. The `sh` repository should not own this lane; it owns interpreter
semantics, not the bashy conformance execution substrate.

### First Implementation PR Boundary

The first slice should be split by contract:

- `doctor fleet --json` owns `HostFacts`, raw diagnostics, and derived
  capabilities.
- The Bash 5.3 conformance lane owns the digest-pinned non-root image recipe,
  in-container helper rebuilds, copied testdata, and hermeticity tests.
- `bashy run` / `dag` / preflight owns `RunRecord` emission, resolved image
  digest recording, closed-enum enforcement, and placement refusal.

One implementation nuance from `opencode`: `doctor fleet --json` and
`HostFacts` should not become one overloaded struct. `doctor` may emit raw
diagnostics, exit codes, timestamps, and explanatory errors; `HostFacts` is the
sanitized capability vector consumed by placement. They can share a common
capabilities subset and be produced in a single pass.

Must not be included in the first implementation slice:

- SQLite projections.
- Leases, resume, retries, or a durable scheduler.
- OTEL ingestion or dashboards.
- Registry mirroring.
- Paired Windows container routing.
- Dynamic matrix expansion.
- Rich DSL/resource semantics.
- Host provisioning fixes for individual private machines.
