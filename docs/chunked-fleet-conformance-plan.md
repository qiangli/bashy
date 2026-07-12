# Chunked Fleet Conformance Plan

Status: design note.

Purpose: define the minimal reliable path for running GNU Bash 5.3 and yash
conformance tests chunked and distributed across a fleet of heterogeneous hosts.

This plan is grounded in the Bashy platform retrospective discussion and carries
forward its agreed first-slice decisions. Source references:

- Retrospective minutes:
  [`docs/meetings/meeting-note-2026-07-11T21-58-bashy-platform-retrospective-fleet-testing-block.md`](meetings/meeting-note-2026-07-11T21-58-bashy-platform-retrospective-fleet-testing-block.md)
- First-slice implementation brief:
  [`docs/bashy-platform-retrospective-implementation-brief.md`](bashy-platform-retrospective-implementation-brief.md)
- Interim meeting notes:
  [`docs/bashy-platform-retrospective-interim-notes.md`](bashy-platform-retrospective-interim-notes.md)
- Original request:
  [`docs/bashy-platform-retrospective-original-request.md`](bashy-platform-retrospective-original-request.md)

Privacy rule: public artifacts must not include private hostnames, usernames,
local paths, LAN addresses, tokens, secret values, or environment-identifying
details. Use neutral role labels such as "local macOS host", "remote Linux
container-capable host", and "remote Windows host".

## Goal

Bashy should be able to run GNU Bash 5.3 and yash conformance suites in chunks
across multiple hosts without confusing infrastructure failures with shell
conformance failures.

The minimal reliable implementation is not a full workflow engine. It is a
small set of contracts:

- prove host capability before placing work;
- run portable conformance suites in a hermetic container-normalized lane;
- keep chunk membership stable;
- emit one structured record for every chunk outcome;
- use timing history only to improve future chunk manifests.

## Non-Goals

Do not include these in the minimal implementation:

- durable workflow engine;
- rich pipeline DSL;
- dynamic matrix expansion;
- retry/backoff engine;
- scheduler leases/resume/replay;
- OTEL dashboards;
- registry mirroring;
- paired-host Windows container routing;
- Slurm, Kubernetes, cloud, or HPC adapters.

## Host Preflight

Add `doctor fleet --json` as the preflight contract. It emits `HostFacts` for
each target host.

Minimum `HostFacts` areas:

- OS, architecture, and kernel;
- bashy version and commit;
- SSH and non-interactive auth status;
- PTY and elevation status;
- OCI availability, provider, rootless support, and version;
- registry reachability and image pull capability;
- required tools, including `bash`, `gcc`, `make`, and `hexdump`;
- disk availability;
- `facts_digest`;
- `observed_at`;
- `ttl_seconds`;
- capability errors.

Capability values are tri-state:

- `true`
- `false`
- `unknown`

`unknown` refuses placement. A chunk must not run on a host whose lane
requirements have not been proven.

## Test Lanes

Define test lanes explicitly.

`container-normalized` is the default for portable conformance suites such as
GNU Bash 5.3 and yash. The goal is to remove irrelevant host differences while
still using fleet capacity.

`baremetal-platform` is for tests that must measure the real host platform:

- Windows path semantics;
- PTY behavior;
- filesystem semantics;
- process/session behavior;
- auth/elevation behavior;
- OS-specific outpost behavior.

Do not mix these lanes implicitly. A test must declare the lane it needs.

## Hermetic Container Lane

The `container-normalized` lane must be hermetic enough that results do not
depend on the host that launched the container.

Minimum requirements:

- image addressed by digest, not mutable tag;
- non-root execution;
- required tools and libraries present;
- required locales present;
- testdata copied into a writable in-container workdir;
- testdata not executed directly from a host bind mount;
- helper binaries rebuilt inside the container;
- host-built helper binaries rejected or ignored;
- negative test proves host-helper leakage fails loudly;
- registry/cache/image availability preflighted before fanout.

For GNU Bash 5.3, helper binaries include at least:

- `recho`
- `zecho`
- `xcase`

For yash, apply the same rule to any helper program or generated executable used
by the test harness.

## Static Chunk Manifests

Chunk membership must be stable. Do not derive test-to-chunk membership from
current fleet capacity.

Use a committed or generated manifest with stable assignments:

```text
suite=bash53 chunk=0 test=array
suite=bash53 chunk=0 test=builtins
suite=bash53 chunk=1 test=execscript
suite=yash chunk=0 test=arith
suite=yash chunk=1 test=redir
```

The scheduler may map chunks to hosts dynamically, but the mapping from test
case to chunk should remain stable until a deliberate rebalance updates the
manifest.

This makes these operations reliable:

- selective rerun;
- failure comparison;
- duration history;
- chunk fingerprinting;
- artifact lookup.

## TaskSpec

Each chunk run should be represented by a minimal `TaskSpec`.

Required fields:

- `task_id`;
- `suite`, such as `bash53` or `yash`;
- `lane`;
- `requires[]`;
- `chunk_manifest`;
- `chunk_id`;
- `cwd`;
- `command`;
- `inputs[]`;
- `outputs[]`;
- `env_allowlist`.

First-slice constraints:

- inputs and outputs are static paths, artifacts, or URIs;
- dependencies are static logical task IDs;
- no dynamic expressions;
- no parameter sweep or `matrix`;
- no general-purpose metadata language;
- `TaskSpec` names a lane but does not carry per-task image digests.

## RunRecord

Every chunk attempt emits a `RunRecord`.

Required fields:

- `run_id`;
- `task_id`;
- `suite`;
- `chunk_id`;
- `host_id`;
- `facts_digest`;
- `status`;
- `exit_code`;
- `started_at`;
- `ended_at`;
- `preflight_failure`;
- `unsatisfied_requirement`;
- `image_digest`;
- `code_sha`;
- `input_digest`;
- `output_digest`;
- log references;
- artifact references.

Preflight refusal is a first-class outcome:

```json
{
  "status": "preflight_failed"
}
```

It is not a skip and not an absent run.

## Preflight Failure Classes

Use dotted lowercase failure classes with closed prefixes:

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

These failures must be reported separately from conformance failures. A host
that cannot run a chunk did not produce evidence that bashy failed GNU Bash or
yash compatibility.

## Timing History And Smart Chunking

Persist per-test duration history so chunk manifests can be improved over time.

Minimum local state:

- append-only event log for raw run records;
- SQLite projection for querying timings and recent failures.

Initial smart chunking loop:

1. Run the current chunk manifest.
2. Store per-test and per-chunk durations.
3. Rebalance the manifest using historical durations.
4. Repeat for a small bounded number of rounds.
5. Save the best manifest for future runs.

The optimization target is wall-clock time, but correctness comes first. A
rebalance must not change what a passing/failing result means.

## Minimal Implementation Order

1. Implement `doctor fleet --json` and `HostFacts`.
2. Add static chunk manifest support for GNU Bash 5.3 and yash.
3. Implement the hermetic `container-normalized` lane.
4. Emit `RunRecord` for every chunk attempt.
5. Enforce preflight placement refusal before execution.
6. Persist duration history.
7. Add bounded duration-based chunk rebalance.

This is sufficient to make distributed GNU Bash 5.3 and yash testing reliable
without committing to the full future platform.
