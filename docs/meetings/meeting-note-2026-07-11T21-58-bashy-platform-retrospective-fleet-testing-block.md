# Meeting — Bashy Platform Retrospective

Date: 2026-07-11

Status: closed after final-call round.

Context:

- Original request:
  [`docs/bashy-platform-retrospective-original-request.md`](../bashy-platform-retrospective-original-request.md)
- Meeting brief:
  [`docs/bashy-platform-retrospective-brief.md`](../bashy-platform-retrospective-brief.md)
- Interim notes:
  [`docs/bashy-platform-retrospective-interim-notes.md`](../bashy-platform-retrospective-interim-notes.md)
- Implementation brief:
  [`docs/bashy-platform-retrospective-implementation-brief.md`](../bashy-platform-retrospective-implementation-brief.md)

Privacy rule: these minutes intentionally omit private hostnames, usernames,
local paths, LAN addresses, tokens, and environment-identifying details.

## Attendees

- `codex`
- `claude`
- `agy`
- `opencode`
- Secretary: `aider`

## Purpose

The meeting reviewed fleet testing blockers from the Bash 5.3 compatibility
work and placed them in the broader bashy roadmap: GNU/POSIX compatibility,
in-process userland, O3 support, venue-aware execution, pipeline/DAG/fanout,
optional DSL/HPC support, and datastore-backed provenance.

## Core Finding

The fleet incident was not a Bash 5.3 semantics failure. It exposed unmodeled
host readiness, substrate, and hermeticity assumptions:

- one remote Windows host lacked non-interactive auth/elevation;
- another remote Windows host lacked a usable OCI substrate through lean bashy;
- one remote Linux container-capable host had registry access failure;
- the local container lane allowed host-built Bash test helper binaries to leak
  into Linux container execution through mounted testdata.

The durable correction is to make bashy model host capability facts, refuse
placement with typed preflight failures, and enforce hermetic lane contracts
before fleet fanout is allowed to report conformance results.

## Decisions

1. First implementation sequence: `schemas-and-both`.
2. Ship the narrow fixes as one first slice:
   - stable `HostFacts`-oriented `doctor fleet --json`;
   - hermetic digest-pinned Bash 5.3 OCI lane;
   - minimal `TaskSpec` and `RunRecord` envelopes sufficient for later durable
     fanout.
3. Use the three-noun model for first-slice execution:
   - `HostFacts`: what a machine can actually do;
   - `TaskSpec`: what a unit of work requires;
   - `RunRecord`: what happened.
4. Preserve Bash compatibility mechanically: no new construct goes into files
   that normal Bash would parse.
5. Keep early bashy metadata declarative and in bashy-owned DAG/sidecar
   surfaces.
6. Use dotted lowercase preflight failure classes with a closed prefix set.
7. Put container image identity in lane definitions. `TaskSpec` names a lane;
   `RunRecord` records the resolved image digest actually used.
8. Put the hermetic Bash 5.3 lane in the bashy repository first, because it is
   part of bashy's conformance harness and release contract.
9. Treat the containerized lane as a fast heterogeneous signal, not a
   replacement for the authoritative serial single-host release gate.
10. Keep raw doctor diagnostics separate from the sanitized `HostFacts`
    scheduler view, even if both are produced in one pass.
11. Apply the privacy scrub rule to public minutes, docs, schema examples, and
    commit messages.

## First-Slice Contracts

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

No dynamic expressions, parameter sweeps, or general-purpose metadata language
belong in the first slice.

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

## Preflight Failure Taxonomy

Use dotted lowercase classes with these frozen prefixes:

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

## Action Items

- Freeze `HostFacts` v1 with dotted lowercase refusal classes and closed
  prefixes.
- Ship `doctor fleet --json` from the `HostFacts` contract, while keeping raw
  diagnostics separate from scheduler facts.
- Implement a digest-pinned, non-root Bash 5.3 OCI lane in the bashy repo.
- Rebuild `recho`, `zecho`, and `xcase` inside the container.
- Copy testdata into a writable in-container workdir instead of executing it
  directly from host mounts.
- Record resolved lane image digests in `RunRecord`.
- Emit `RunRecord` for success, conformance failure, substrate failure, and
  preflight refusal.
- Enforce capability-based preflight placement refusal before chunk execution.
- Persist first-slice run/provenance data in local SQLite as the first
  projection, while keeping the append-only event log as the conceptual source
  of truth.
- Keep the authoritative serial single-host Bash 5.3 release gate separate from
  the chunked container lane.
- Apply the privacy scrub rule to docs, schema examples, minutes, briefs, and
  commit messages before publication.

## Deferred Work

- Durable workflow engine.
- Rich pipeline DSL.
- Retry/backoff semantics.
- Scheduler leases, resume, and replay.
- OTEL ingestion and dashboards.
- Local registry mirroring.
- Paired-host Windows container routing.
- Dynamic matrix expansion.
- Slurm, Kubernetes, cloud, or HPC adapters.
- Host provisioning fixes for individual private machines.

## Residual Risks

- Host capability schemas do not catch every hermeticity defect. The Bash 5.3
  lane needs explicit tests for host-helper leakage.
- The chunked container lane could drift into being treated as the certification
  gate. The serial single-host gate must remain authoritative unless policy
  changes explicitly.
- `doctor fleet --json` and `HostFacts` can drift if one is treated as a
  presentation format and the other as a separate scheduler format. They should
  share a common capabilities subset produced in one pass.
- Public artifacts can leak private environment details unless scrubbed before
  commit and publication.

## Closure

The final-call round produced agreement that the meeting can conclude. The
meeting may be resumed later if implementation evidence reveals new design
questions.

