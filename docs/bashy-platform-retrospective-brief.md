# Bashy Platform Retrospective Brief

Date: 2026-07-11

Source request: this brief is derived from the initiating request preserved in
[`docs/bashy-platform-retrospective-original-request.md`](bashy-platform-retrospective-original-request.md).
That file keeps the user's original framing with only light grammar, syntax,
capitalization, and typo corrections.

Purpose: shared context for a `bashy meet` retrospective on fleet testing
blockers and the broader bashy platform direction. This is not only about fixing
the immediate Bash 5.3 fleet run. The larger question is how bashy should grow
so containerized heterogeneous testing, baremetal platform coverage, pipeline
orchestration, agentic execution, HPC/DSL workflows, and datastore-backed
provenance become ordinary first-class features rather than recurring bespoke
scripts.

Privacy note: public artifacts should avoid private hostnames, usernames, local
paths, LAN addresses, tokens, and environment-identifying details. This document
uses role-based host labels such as "local macOS host" and "remote Windows host"
instead of private machine names.

## Immediate Incident

We tried to run the GNU Bash 5.3 compatibility suite over the development fleet:

- local macOS host
- remote Linux container-capable host
- remote Windows host A
- remote Windows host B

The current result is a useful failure, not a valid fleet pass.

Observed blockers:

- Remote Windows host A cannot currently be driven non-interactively: SSH
  elevation requires a TTY for the password prompt.
- Remote Windows host B has no usable container substrate through the current
  lean bashy:
  `bashy podman: no podman found on PATH or in bashy's cache`.
- The remote Linux container-capable host has a working `./bin/bashy podman
  info`, but the actual
  chunk run failed to pull `gcc:14-bookworm` because its registry proxy refused
  connections.
- Local container chunks exposed that mounting the host Bash 5.3 testdata
  directory directly is not sufficient. The mounted tree reused host-built helper
  programs such as `recho`, `zecho`, and `xcase`, which then failed inside the
  Linux container with `cannot execute binary file`.
- Some Bash tests also reject running as root inside the container
  (`new-exp`, `test`), so the container lane must run as a normal user.
- The container image currently lacks some utilities expected by the Bash tests,
  for example `hexdump`.

One DAG issue was fixed during the attempt:

- `test-bash-container-prepare` no longer forces the heavy `test-podman` engine
  rebuild. It now validates and uses the current `bashy podman` surface. Commit:
  `b3adea0 test: use bashy podman for container bash chunks`.

Important framing:

- Generic conformance suites should default to a container-normalized lane.
- Baremetal platform lanes remain mandatory when the platform itself is the
  coverage subject: Windows path semantics, PTY behavior, filesystem semantics,
  process/session behavior, and OS-specific outpost behavior.

## Existing Bashy Foundation

Bashy is already more than a shell wrapper. Current code and docs describe these
foundational layers:

- GNU Bash 5.3 compatible pure drop-in binary: `cmd/bash`.
- AgentOS shell binary: `cmd/bashy`, wiring shell core plus coreutils and
  agentic verbs.
- POSIX conformance work in progress, with yash POSIX suite as an external
  frontier metric.
- In-process coreutils and classic Unix surface. `bashy commands` currently
  reports 193 builtins, including shell builtins, coreutils, and classic tools.
- Managed external tools, currently including `git`, `gh`, `go`, `rg`, `kubectl`,
  `helm`, `rclone`, `kopia`, `zot`, `seaweedfs`, language toolchains, and more.
- Agentic/workspace verbs: `dag`, `weave`, `sprint`, `meet`, `fanout`,
  `foreman`, `schedule`, `secrets`, `doctor`, `commands`, `context`, `kb`,
  `skills`, `run`, `self`, `check`, and `verify`.
- Sandbox/sphere/account verbs: `podman`, `docker`, `ollama`, `sphere`, `login`,
  and `tessaro`.

The strategic point: bashy already spans command compatibility, managed tooling,
agentic orchestration, containers, local scheduling, secrets, and meeting/fanout
coordination. The defects we hit are not evidence that the direction is wrong;
they show where the platform needs harder contracts and better substrate
management.

## Three Pillars: O3 Embedded

The working product model is:

- OCI: `bashy podman` / `bashy docker` as the reproducible execution substrate.
- Ollama: local managed model serving for agentic and offline workflows.
- OTEL: OpenTelemetry/Victoria/related observability substrate for traces,
  metrics, logs, dashboards, and run telemetry.

The O3 pillar implication for this incident:

- OCI cannot be an optional convenience if container-normalized testing is the
  default for heterogeneous fleets.
- OTEL cannot be only an app stack; pipeline/fleet execution should emit traces,
  metrics, logs, artifact metadata, retry events, and host/substrate diagnostics.
- Ollama and local model routing become relevant when meeting/fanout/foreman
  tasks need cheap local triage before premium agents are assigned.

## Venues / Strata

Bashy should model execution venue explicitly:

- `userland`: single shell/session, in-process commands, local user tools.
- `workspace`: repo checkout, DAG, weave workspaces, source/test artifacts.
- `sandbox`: containerized execution and controlled mounts.
- `sphere`: local machine services, Ollama, OCI machines, observability stack.
- `cluster`: multiple hosts, SSH/mesh, schedulers, resource pools.
- `cloud`: remote object stores, managed runners, Kubernetes, cloud APIs.

Each task should declare requirements in those terms, for example:

- `requires: container`
- `requires: baremetal-windows`
- `requires: pty`
- `requires: nonroot-container`
- `requires: gcc`
- `requires: hexdump`
- `requires: registry-cache:bash53-gcc`
- `requires: datastore`
- `requires: otel`

The scheduler should then place work only on hosts whose capability facts match.
The recent failure happened partly because capability reality was implicit.

## Pipeline State of the Art

External workflow systems converge on a few ideas bashy should internalize.

Temporal emphasizes durable execution: workflows are ordinary code whose state is
recoverable, replayable, pauseable, and resilient to crashes. See Temporal's
overview of durable execution and workflow execution:
https://temporal.io/ and https://docs.temporal.io/workflow-execution.

Nextflow focuses on scalable, reproducible scientific workflows with containers,
Git collaboration, cloud, and HPC portability. See:
https://www.nextflow.io/.

Snakemake describes reproducible and scalable analyses with software
environment deployment and the ability to run on local, server, cluster, grid,
and cloud backends without changing the workflow definition. See:
https://snakemake.readthedocs.io/.

Argo Workflows is container-native and Kubernetes-native; each step is normally
a container and workflows can be steps or DAGs. See:
https://argoproj.github.io/workflows/.

Dagster's useful distinction is asset-centric orchestration: it tracks produced
assets, lineage, health, and blast radius, not only task success. See:
https://dagster.io/.

Prefect's useful emphasis is reliable flow execution on user infrastructure:
task tracking, retries, recovery, and scaling. See:
https://www.prefect.io/ and
https://docs.prefect.io/v3/how-to-guides/workflows/retries.

OpenTelemetry provides the vendor-neutral standard for traces, metrics, and
logs. See: https://opentelemetry.io/docs/.

Martian is directly relevant to HPC-style pipelines. Its language/framework
supports typed pipeline stages with `in` and `out` parameters and stage code in
virtually any language, using JSON files as the stage boundary. See:
https://martian-lang.org/writing-pipelines/ and
https://martian-lang.org/writing-stages/.

## Pipeline Requirements for Bashy

To become a robust pipeline substrate rather than a make replacement, bashy DAG
and adjacent verbs need first-class support for:

- DAG definition and execution.
- Explicit task capabilities and venue requirements.
- Orchestration and scheduling across time, events, upstream completion, and CI
  failure webhooks.
- Reproducibility through versioned code, data, images, toolchain pins, and
  environment manifests.
- Scalability across local cores, multiple hosts, containers, clusters, and
  cloud backends.
- Fault tolerance: retries, backoff, resume from checkpoint, idempotency gates,
  leases, stale-run detection, cancellation, and recovery.
- Provenance: inputs, outputs, command line, env digest, code SHA, image digest,
  host facts, timing, logs, and produced artifacts.
- Monitoring: live status, per-task logs, traces, metrics, and dashboards.
- Infrastructure abstraction: local process, container, remote host, Slurm,
  Kubernetes, or cloud runner should be backend choices, not workflow rewrites.

The existing `dag`, `fanout`, `schedule`, `meet`, `weave`, `foreman`, `run`,
`doctor`, `secrets`, and `commands` surface is a strong starting point. The gap
is the uniform control plane that makes these composable.

## HPC / DSL Direction

Bash itself is already a programming language: it has syntax, semantics,
variables, control flow, functions, and Turing completeness. Bashy's opportunity
is to preserve Bash compatibility while adding opt-in language-level constructs
for reliable orchestration.

Potential Bashy-native constructs:

- Declarative metadata blocks for tasks:
  `# bashy: requires container, gcc, nonroot`
- Typed task inputs and outputs inspired by Martian:
  `in`, `out`, `file`, `dir`, `json`, `artifact`, `secret`.
- First-class artifact references instead of ad hoc paths.
- First-class dataset/image/toolchain pins.
- Pipeline-level `scatter`, `gather`, `matrix`, and duration-aware chunking.
- Resource declarations: CPU, memory, disk, GPU, wall time, network, filesystem
  mount policy.
- Venue selectors: local, sandbox, sphere, cluster, cloud.
- Backend adapters: local, bashy podman, SSH mesh, Slurm, Kubernetes/Argo,
  cloud batch.

Design constraint:

- GNU Bash compatibility must remain the default. New constructs should be
  opt-in, likely through `bashy dag` metadata, comments, explicit `bashy` blocks,
  or separate `.bashy.md` / `.bpipe.md` files rather than silently changing Bash
  semantics.

## Datastore / Provenance Direction

Durable orchestration needs a datastore. Today many bashy features rely on files
and local stores. That is good for bootstrap, but the platform should expose a
consistent datastore abstraction.

Recommended layers:

- Local embedded store: SQLite for task state, leases, artifacts, host facts,
  schedules, secrets metadata, and provenance.
- Object/blob store: file tree, S3-compatible storage, or SeaweedFS for logs,
  artifacts, images, and large test outputs.
- Graph/knowledge store: existing `graph` / `kb` style memory for code and
  operational findings.
- Vector/semantic index: optional retrieval layer for logs, docs, failures, and
  knowledge base content.
- Metrics/time-series: Victoria/OTEL path for durations, retries, failures,
  resource usage, and fleet health.

Minimum tables/entities:

- `hosts`: OS, arch, labels, capabilities, container provider, auth mode,
  last-seen, health.
- `tasks`: logical target, code SHA, input digest, output digest, status,
  retries, timing.
- `runs`: DAG/fanout execution instances, parent-child edges, conductor/agent.
- `artifacts`: files, directories, images, logs, result envelopes, checksums.
- `leases`: claimed work, expiry, owner, heartbeat.
- `events`: append-only trace of decisions, retries, failures, recovery actions.
- `secrets_refs`: references only, never secret values.

This would make a failed fleet run queryable:

- Which host failed?
- Was it substrate, network, missing tool, image pull, test failure, or auth?
- Which chunk was assigned?
- Which image digest and helper binaries were used?
- Can the task resume after substrate remediation?

## Perfect Solution Sketch for the Current Blockers

The blockers from the fleet run should become impossible or self-diagnosing:

1. Host capability inventory
   - `bashy doctor fleet --json` gathers facts: OS, arch, `bashy --version`,
     commit, sibling pins, container provider, image cache, registry reachability,
     non-interactive SSH, elevation mode, PTY support, toolchain, and disk.
   - `bashy capability` stores/updates host capability facts.

2. Container substrate manager
   - `bashy podman install/path/doctor` becomes complete and cross-platform.
   - Windows supports WSL2 or Hyper-V provider, or explicitly pairs to a host
     node for container work.
   - Registry images can be mirrored into `zot` or another local registry so
     fleet runs do not depend on Docker Hub/proxy availability.

3. Hermetic test images
   - Bash 5.3 compat tests run in a dedicated image, not raw `gcc:14-bookworm`.
   - Image includes `gcc`, `make`, `hexdump`, locales, non-root user, and any
     required helper build dependencies.
   - Testdata is copied into an in-container writable workdir; helper binaries
     are rebuilt inside the image; no host-built artifacts leak in.
   - Test image is addressed by digest and cached/mirrored before fanout starts.

4. Capability-aware placement
   - Generic Bash 5.3 compat: `requires: container, bash53-image`.
   - Windows path/PTY coverage: `requires: baremetal-windows, pty`.
   - Scheduler refuses to place a task on a host that lacks the requirement,
     and reports the missing capability rather than running and failing late.

5. Durable fanout
   - `dag-fanout` becomes a proper scheduler primitive with leases, retries,
     per-chunk logs, resume, cancellation, and provenance.
   - Smart chunking persists duration profiles and plan quality.
   - Failed chunks are resumable on another capable host.

6. Observability
   - Every task emits a `bashy-run-v1` envelope plus OTEL spans.
   - The conductor sees a dashboard: host prep, image pulls, chunk start/end,
     failures, retries, and aggregate result.

7. Meeting-to-execution loop
   - `meet` decides and records.
   - `sdlc` turns decisions into work packages.
   - `weave` executes code changes in isolated workspaces.
   - `dag`/`fanout` verifies.
   - `kb`/datastore records lessons so the next conductor does not rediscover
     the same substrate issue.

## Open Questions for the Meeting

1. Should bashy build a full durable workflow engine, or extend `dag`/`fanout`
   incrementally until it reaches that point?
2. What is the right default datastore: SQLite first, or file-backed event log
   with SQLite projections?
3. Should the Bashy DSL live in markdown DAG metadata, Bash comments, a new
   `.bpipe` syntax, or a Martian-like separate language?
4. How much Kubernetes/Slurm/cloud support should be built in versus delegated
   through adapters?
5. Should every test suite have two declared lanes: `container-normalized` and
   `baremetal-platform`, with no implicit default?
6. How should Windows hosts participate in container-normalized workloads:
   native Hyper-V/WSL provider, remote paired host, or both?
7. Should bashy ship curated hermetic conformance images, or build them locally
   from declarative recipes?
8. What are the minimum OTel signals and datastore events needed for the next
   fleet run?
9. How do we keep GNU/POSIX compatibility pristine while adding new language
   constructs?
10. What should be implemented first so the recent blocker list becomes a
    one-command diagnosed/remediated condition?

## Proposed Meeting Agenda

1. Retrospective: what the fleet failure actually taught us.
2. Target architecture: bashy as shell, userland, orchestrator, and substrate.
3. Pipeline/DAG/fanout enhancements and durable execution semantics.
4. Container substrate and hermetic test images.
5. Baremetal platform coverage and host capability inventory.
6. HPC/DSL direction and compatibility boundaries.
7. Datastore/provenance/OTEL plan.
8. Prioritized implementation roadmap and owners.

## Suggested First-Round Prompts

- `codex-gpt-5.5`: propose the minimal architecture that makes the recent
  fleet blockers structurally impossible while preserving Bash compatibility.
- `claude-opus`: critique product scope and identify the smallest coherent
  feature set that users/agents will understand.
- `agy-gemini3.1`: focus on pipeline/HPC/DSL design and compare with Martian,
  Nextflow, Snakemake, Temporal, and Argo.
- `opencode-deepseek-v4`: focus on implementation risks, code organization,
  and how to incrementally evolve current `dag`, `fanout`, `podman`, `doctor`,
  and datastore surfaces.
