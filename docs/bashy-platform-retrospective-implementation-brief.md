# Bashy Fleet Testing First-Slice Implementation Brief

Date: 2026-07-11

Status: draft, pending meeting closure.

Source material:

- Original request:
  [`docs/bashy-platform-retrospective-original-request.md`](bashy-platform-retrospective-original-request.md)
- Meeting brief:
  [`docs/bashy-platform-retrospective-brief.md`](bashy-platform-retrospective-brief.md)
- Interim notes:
  [`docs/bashy-platform-retrospective-interim-notes.md`](bashy-platform-retrospective-interim-notes.md)

Privacy rule: public artifacts must not include private hostnames, usernames,
local paths, LAN addresses, tokens, secret values, or environment-identifying
details. Use neutral role labels such as "local macOS host", "remote Linux
container-capable host", and "remote Windows host".

## Decision Summary

The first implementation slice is `schemas-and-both`:

- Ship a stable `HostFacts`-oriented `doctor fleet --json` contract.
- Ship the hermetic digest-pinned Bash 5.3 container lane.
- Emit minimal `TaskSpec` and `RunRecord` envelopes sufficient for later
  durable fanout.

This slice must not build the full datastore, durable scheduler, OTEL
dashboards, Slurm/Kubernetes/cloud adapters, pipeline DSL, or paired Windows
container routing.

## First-Slice Contracts

### HostFacts

`HostFacts` answers: what can this host actually do?

Schema identity:

```json
{
  "schema": "bashy.hostfacts.v1",
  "schema_version": 1
}
```

Minimum fields:

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

Capability values are tri-state: `true`, `false`, or `unknown`. `unknown` is a
placement refusal, not a permissive default.

Versioning: `bashy.hostfacts.v1` is additive-only. Consumers ignore unknown
fields. Removing, renaming, or changing the semantics of a field requires `v2`.

### TaskSpec

`TaskSpec` answers: what does this unit of work require?

Minimum fields:

- `schema: "bashy.taskspec.v1"`
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
- `fanout.chunk_manifest`, or static `fanout.chunk_id` and `fanout.total`

Constraints:

- Inputs and outputs are static literal workspace paths, artifacts, or URIs.
- Dependencies are static logical `task_id` values.
- No dynamic expressions.
- No general-purpose metadata language.
- No `matrix` or parameter sweep in the first slice.
- `TaskSpec` names a lane; it does not own per-task image digests.

### RunRecord

`RunRecord` answers: what happened?

Schema identity:

```json
{
  "schema": "bashy.run.v1",
  "schema_version": 1
}
```

Minimum fields:

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
- `preflight_failures[]`
- `unsatisfied_requirement`
- `image_digest`
- `code_sha`
- `input_digest`
- `output_digest`
- `log_ref` or `log_paths[]`
- `artifact_refs[]`

Placement refusal is recorded as a `RunRecord` with
`status: "preflight_failed"`. It must not be represented as a skip or absent
run.

## Preflight Failure Taxonomy

Use dotted lowercase failure classes with a frozen prefix set:

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

Initial classes:

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

Every preflight failure carries `host_id` and `unsatisfied_requirement`.
Substrate, auth, host, image, and tool failures must never be reported as Bash
conformance failures.

## Bash 5.3 Container Lane

The hermetic Bash 5.3 lane lives in the bashy repository because it is part of
the conformance harness and release contract. Reusable OCI plumbing can move to
coreutils after a second consumer exists.

Acceptance criteria:

- Image is addressed by digest, not mutable tag.
- The resolved image digest is written to `RunRecord`.
- Run uses a non-root user.
- Image includes required test tools and libraries, including `gcc`, `make`,
  `hexdump`, and required locales.
- Testdata is copied into a writable in-container workdir.
- Testdata is not executed directly from a host bind mount.
- `recho`, `zecho`, and `xcase` are rebuilt inside the container.
- Host-built executable helper artifacts are rejected or ignored.
- A negative test proves host-helper leakage fails loudly.
- Registry/cache/image availability is preflighted before fanout.
- The lane reaches the expected Bash 5.3 compatibility result on a Linux SUT.
- The lane produces byte-identical results from a local macOS host and a remote
  Linux container-capable host.

Non-goal: this container lane does not replace the authoritative serial
single-host release gate unless that policy is changed explicitly later.

## Implementation Ownership

Split the first slice by contract:

- `doctor fleet --json`: owns raw diagnostics, derived `HostFacts`, tri-state
  capabilities, `facts_digest`, and stale-facts handling.
- Bash 5.3 lane: owns digest-pinned image recipe, non-root execution,
  in-container helper rebuilds, copied testdata, and hermeticity tests.
- `bashy run` / `dag` / preflight: owns `TaskSpec` parsing, placement refusal,
  closed-enum enforcement, and `RunRecord` emission.

Keep raw doctor diagnostics separate from the sanitized `HostFacts` scheduler
view. They can be produced in one pass and share a common capabilities subset,
but placement should not consume presentation-oriented diagnostic fields.

## Deferred Work

Do not include these in the first implementation PR:

- SQLite projections.
- Leases, resume, retries, or durable scheduler state.
- OTEL ingestion, metrics, or dashboards.
- Registry mirroring.
- Paired-host Windows container routing.
- Dynamic matrix expansion.
- Rich DSL or typed resource language.
- Slurm, Kubernetes, cloud, or HPC adapters.
- Host provisioning fixes for individual private machines.

