# Bashy Platform Retrospective: Draft Minutes

Date: 2026-07-11

Status: superseded by closed meeting minutes:
[`docs/meetings/meeting-note-2026-07-11T21-58-bashy-platform-retrospective-fleet-testing-block.md`](meetings/meeting-note-2026-07-11T21-58-bashy-platform-retrospective-fleet-testing-block.md).

References:

- Original request:
  [`docs/bashy-platform-retrospective-original-request.md`](bashy-platform-retrospective-original-request.md)
- Meeting brief:
  [`docs/bashy-platform-retrospective-brief.md`](bashy-platform-retrospective-brief.md)
- Interim notes:
  [`docs/bashy-platform-retrospective-interim-notes.md`](bashy-platform-retrospective-interim-notes.md)
- Implementation brief:
  [`docs/bashy-platform-retrospective-implementation-brief.md`](bashy-platform-retrospective-implementation-brief.md)

Privacy rule: these minutes intentionally use neutral host labels and omit
private hostnames, usernames, local paths, LAN addresses, tokens, and
environment-identifying details.

## Participants

- `codex`
- `claude`
- `agy`
- `opencode`
- Secretary: `aider`

## Purpose

The meeting was convened to review fleet testing blockers from the Bash 5.3
compatibility effort and to place those blockers in the broader bashy platform
roadmap: Bash/POSIX compatibility, in-process userland, O3 support, venue-aware
execution, pipeline/DAG/fanout, optional DSL/HPC features, and datastore-backed
provenance.

## Core Finding

The fleet failure was not a Bash 5.3 semantics failure. The failures exposed
unmodeled host readiness, substrate, and hermeticity assumptions:

- non-interactive auth/elevation was not available on one remote Windows host;
- OCI substrate was not available through lean bashy on another remote Windows
  host;
- registry access failed on a remote Linux container-capable host;
- host-built Bash test helper binaries leaked into a Linux container through
  mounted testdata.

The durable lesson is that bashy needs capability facts, typed preflight
refusals, and hermetic lane contracts before fleet fanout can produce
trustworthy conformance results.

## Decisions

### First Implementation Sequence

Decision: use `schemas-and-both`.

The first slice includes:

- stable `HostFacts`-oriented `doctor fleet --json`;
- hermetic digest-pinned Bash 5.3 OCI lane;
- minimal `TaskSpec` and `RunRecord` envelopes sufficient for later durable
  fanout.

The first slice intentionally does not include a full datastore, durable
scheduler, OTEL dashboarding, Slurm/Kubernetes/cloud adapters, paired Windows
container routing, or a pipeline DSL.

### Architecture Model

The meeting converged on three core nouns:

- `HostFacts`: what a machine can actually do;
- `TaskSpec`: what a unit of work requires;
- `RunRecord`: what happened, append-only.

Placement is a join of `TaskSpec` requirements against `HostFacts`. Adapters
consume `TaskSpec` and emit `RunRecord`.

### Compatibility Boundary

No new construct should appear in a file that normal Bash would parse. Bashy
metadata belongs in markdown DAG headings, `.bashy.md` sidecars, or other
bashy-owned descriptors. Early metadata is declarative only; no loops,
conditionals, dynamic expressions, or general-purpose metadata language.

### Venue Model

The venue vocabulary remains:

- `userland`
- `workspace`
- `sandbox`
- `sphere`
- `cluster`
- `cloud`

Only `userland`, `workspace`, and `sandbox` should receive first-slice runtime
semantics. The remaining venues stay vocabulary and adapter direction until real
consumers justify them.

### O3 Framing

OCI is load-bearing for sandbox execution and container-normalized testing.

OTEL should initially be an export/view of `RunRecord`, not primary state.

Ollama is valuable for agentic triage and local workflows, but should not be in
the scheduler critical path.

### Datastore Direction

The preferred state model is:

- append-only file-backed event log as source of truth;
- SQLite as a rebuildable projection later;
- OTEL as export/view;
- object/blob storage later for large logs and artifacts;
- graph/KB/vector layers later as query aids.

## First-Slice Interface Decisions

### HostFacts

`HostFacts` uses schema identity `bashy.hostfacts.v1` and additive-only
versioning. Capability values are tri-state: `true`, `false`, or `unknown`.
`unknown` refuses placement.

Required areas include:

- host identity and observation time;
- `facts_digest` and TTL;
- bashy version/commit;
- platform OS/arch/kernel;
- auth/PTY/elevation capabilities;
- OCI provider, rootless mode, registry reachability, and cached images;
- required tools such as `bash`, `gcc`, `make`, and `hexdump`;
- disk availability;
- labels and errors.

### TaskSpec

`TaskSpec` uses schema identity `bashy.taskspec.v1`.

Required areas include:

- `task_id`;
- `lane`;
- `venue`;
- static `requires[]`;
- static `inputs[]` and `outputs[]`;
- static `depends_on[]`;
- `cwd`;
- `command`;
- `env_allowlist`;
- static fanout chunk metadata or a committed chunk manifest reference.

`TaskSpec` names a lane. It does not carry per-task image digests in v1.

### RunRecord

`RunRecord` uses schema identity `bashy.run.v1`.

Required areas include:

- run, parent, task, attempt, and host identity;
- `facts_digest`;
- status and timing;
- exit code;
- preflight failure class and unsatisfied requirement;
- resolved image digest;
- code/input/output digests;
- log and artifact references.

Placement refusal emits a `RunRecord` with `status: "preflight_failed"`.
Refusal is not a skip and not an absent run.

### Preflight Failure Taxonomy

Use dotted lowercase classes with frozen prefixes:

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

Initial classes include:

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

Substrate, auth, image, tool, and host-readiness failures must never be reported
as Bash conformance failures.

## Bash 5.3 Hermetic Lane

The Bash 5.3 hermetic OCI lane belongs in the bashy repository first because it
is part of bashy's conformance and release contract.

Acceptance criteria:

- image addressed by digest;
- resolved digest recorded in `RunRecord`;
- non-root execution;
- required tools and locales present;
- testdata copied into a writable in-container workdir;
- testdata not executed directly from a host bind mount;
- `recho`, `zecho`, and `xcase` rebuilt inside the container;
- host-built helper artifacts rejected or ignored;
- negative test proves host-helper leakage fails loudly;
- registry/cache/image availability preflighted before fanout;
- expected Bash 5.3 compatibility result on a Linux SUT;
- byte-identical results from a local macOS host and remote Linux
  container-capable host.

The container lane does not replace the authoritative serial single-host
release gate unless that policy is explicitly changed later.

## Action Items

- Draft implementation brief from the converged decisions. Owner: secretary.
- Implement `doctor fleet --json` with raw diagnostics plus derived
  `HostFacts`. Owner: first-slice implementation.
- Implement the Bash 5.3 hermetic OCI lane in bashy. Owner: first-slice
  implementation.
- Emit `RunRecord` for success, failure, and preflight refusal. Owner:
  first-slice implementation.
- Enforce dotted lowercase preflight classes and placement refusal. Owner:
  first-slice implementation.
- Keep public artifacts scrubbed of private environment details. Owner: all
  participants and commit authors.

## Deferred Work

- SQLite projection.
- Durable scheduler, leases, resume, and retries.
- OTEL ingestion and dashboards.
- Local registry mirroring.
- Paired-host Windows container routing.
- Dynamic matrix expansion.
- Rich DSL/resource language.
- Slurm/Kubernetes/cloud/HPC adapters.
- Host provisioning fixes for individual private machines.

## Closure State

The final-call round produced agreement that the meeting could conclude. The
meeting was closed after that round and may be resumed later if implementation
evidence reveals new design questions.
