# Portable pipeline and run evidence

`dhnt.pipeline/v1` and `dhnt.run/v1` are Bashy's first stable, portable
pipeline/evidence contracts. They are plain JSON and contain no Argo,
Kubernetes, or storage-provider types. The public Go API is
`github.com/qiangli/bashy/dhnt`; the worker and gate CLI is `bashy dhnt`.

This slice deliberately does not add signing. A trusted collector must obtain
records through an authenticated execution boundary and validate them before a
privileged broker authors release refs. A Kubernetes `Succeeded` phase is not
evidence by itself.

## `dhnt.pipeline/v1`

A pipeline has these required fields:

- `schema`: exactly `dhnt.pipeline/v1`;
- `pipeline`: stable pipeline identity;
- `source`: immutable `repository`, `commit`, and source-tree `sha256`;
- `tasks`: task identity, closed `lane`, required closed `distribution`,
  dependencies, argv, repository-relative working directory, and literal
  non-secret environment;
- `matrix`: one or more exact task/platform requirements, each with the expected
  named input and output SHA-256 digests.

Lanes are exactly `native`, `container`, `cluster`, and `cloud`. Platforms use
the closed v1 backend set `local`, `vk-native`, `vk-podman`, `k3s`, and `cloud`;
OS is `linux`, `darwin`, or `windows`; architecture is `amd64` or `arm64`.
Every task must occur in the matrix. A native task must use `vk-native`.
Dependencies must exist and be acyclic.

Distribution is exactly `single`, `shardable`, `replicated`, or
`topology-coupled`. It declares execution shape, not executor capability:
accepting a pipeline does not claim that a local, DKS, or cloud executor can
honor every shape.

A `shardable` task must carry an explicit `chunk` object on every matrix entry:
one-based `index`, positive `count`, and the lowercase SHA-256 of the immutable
manifest that defines membership. All platform rows for the same task must
carry the same chunk identity. The shard is therefore part of the task's stable
identity; a pipeline represents multiple shards as distinct task IDs. Fleet
capacity may change how many shard tasks run concurrently, but it must never
derive or renumber chunks. Non-shardable tasks reject `chunk`. `replicated` and
`topology-coupled` are declarations only in v1; this contract does not imply
replica management, gang scheduling, GPU topology discovery, or multi-host
training.

The matrix is an explicit list rather than an implicit cross product. This
allows Linux, Darwin, and Windows release archives to carry different digests
while still requiring exact three-platform coverage.

The byte-stable reference encoding is
[`dhnt/testdata/pipeline.golden.json`](../dhnt/testdata/pipeline.golden.json).

## `dhnt.run/v1`

A run record has these required fields:

- `schema`, `pipeline`, `task`, and unique `run` identity;
- the exact source repository, commit, and source-tree SHA-256;
- one or more named input SHA-256 digests;
- observed executor node, backend, OS, and architecture;
- `result.class` and integer `result.exitCode`;
- one or more named output SHA-256 digests;
- UTC `startedAt` and `finishedAt` in RFC3339/RFC3339Nano form;
- a non-zero, 32-lowercase-hex trace ID.

Result classes are exactly `pass`, `test-fail`, `infra-fail`, `incomplete`, and
`canceled`. `pass` requires exit code zero; `test-fail` requires a non-zero exit
code. `finishedAt` cannot precede `startedAt`.

The byte-stable reference encoding is
[`dhnt/testdata/run.golden.json`](../dhnt/testdata/run.golden.json).

## Typed artifacts and atomic output-set evidence in v2

`dhnt.pipeline/v2` and `dhnt.run/v2` preserve the v1 task, matrix, executor,
result, and source shapes while making artifact identity self-describing. Every
v2 artifact adds required closed fields:

```json
{
  "name": "dataset",
  "kind": "tree",
  "digestAlgorithm": "sha256-tree-v1",
  "sha256": "<64 lowercase hex>"
}
```

The only valid kind/algorithm pairs are:

- `file` / `sha256-file-v1`: raw SHA-256 of the regular file bytes;
- `tree` / `sha256-tree-v1`: the portable canonical tree digest below.

V1 artifacts must omit both new fields, and v2 artifacts must include them.
Aggregation rejects v1 run evidence for a v2 pipeline and vice versa. Existing
v1 decoding, validation, and canonical bytes remain unchanged.

### `sha256-tree-v1`

Tree hashing recursively includes regular files only. Symlinks and special
files are errors and are never followed. Each entry path must be valid UTF-8,
Unicode NFC, clean, slash-relative, non-empty, and free of backslashes and
traversal. Entries are sorted by their UTF-8 bytes. The digest ignores file and
directory modes, mtime, uid, gid, hard-link identity, and empty directories.
Consequently, an empty tree has one stable identity and trees that differ only
in excluded metadata have the same identity.

The root digest is SHA-256 over this exact binary encoding:

```text
"dhnt.sha256-tree/v1\0"
uint64be(file count)
repeat in bytewise path order:
    uint64be(path byte length)
    path UTF-8 bytes
    raw 32-byte SHA-256(file bytes)
```

The domain prefix distinguishes a tree root from a raw file digest. Fixed-width
counts and length-prefixed paths make entry and path boundaries unambiguous.
The public Go entry point is `dhnt.HashArtifact(path, kind, algorithm)`.

### Output commit manifest

A `dhnt.run/v2` record additionally requires `outputCommit`:

```json
{
  "schema": "dhnt.output-commit/v1",
  "digestAlgorithm": "sha256-output-commit-v1",
  "sha256": "<64 lowercase hex>"
}
```

Its SHA-256 is computed over the exact newline-terminated canonical JSON
returned by `MarshalOutputCommitManifest(outputs)`. That manifest has schema
`dhnt.output-commit/v1` and the complete, name-sorted v2 `outputs` array.
Validation recomputes the identity, so changing, removing, or adding any output
invalidates the run.

For a filesystem runner, individual verified outputs may be staged and renamed
first, but consumers must treat them as uncommitted until the manifest itself
is atomically published. The manifest is the single visibility/commit point
for the set; this avoids falsely claiming that multiple filesystem renames form
one transaction. A future runner binding must declare its final manifest path,
reject path and symlink aliases, and publish the manifest last.

The byte-stable v2 reference encodings are
[`pipeline-v2.golden.json`](../dhnt/testdata/pipeline-v2.golden.json),
[`run-v2.golden.json`](../dhnt/testdata/run-v2.golden.json), and
[`output-commit-v1.golden.json`](../dhnt/testdata/output-commit-v1.golden.json).

## Strictness and deterministic encoding

All SHA-256 values are lowercase 64-hex. Unknown and duplicate JSON fields,
multiple JSON values, unknown enum values, duplicate task/artifact/matrix
identities, missing distributions, invalid chunk identities, invalid
timestamps, and malformed identifiers are rejected.

Canonical encoders sort semantically unordered tasks, dependencies,
environment entries, matrix entries, artifacts, and aggregate run identities.
Struct field order and compact JSON output are stable and end with one newline.
Input values are copied before sorting.

`bashy dhnt` exposes:

```text
validate-pipeline [FILE|-]
canonicalize-pipeline [FILE|-]
validate-run [FILE|-]
canonicalize-run [FILE|-]
emit-run FLAGS
aggregate --pipeline FILE RUN...
lower-argo --binding FILE [PIPELINE|-]
```

Aggregation accepts exactly one valid, passing run for each declared matrix
entry. It rejects missing, duplicate, extra, malformed, mismatched, or non-pass
evidence. It compares pipeline/task identity, full source identity, exact named
input and output digests, and observed backend/OS/architecture. The DKS result
collector additionally compares the record's executor fields with the selected
Pod node and its live labels. The Bashy release gate also applies its independent
`linux darwin windows` policy to the declared matrix, so supplying a valid but
weaker one-platform plan cannot authorize three QA refs.

## DKS compatibility boundary

The native producer now requires `PIPELINE_ID`, `EVIDENCE_TASK`,
`SOURCE_REF`, `SOURCE_SHA256`, `RUN_ID`, `TRACE_ID`, and `EXECUTOR_NODE`, and
uses the release archive checksum as the `candidate` input and
`tested-candidate` output digest. The exact plan is supplied separately as
`PIPELINE_FILE` to the release gate. The trusted QA-ref author also requires
`EXPECTED_SOURCE_SHA256` and `PIPELINE_FILE`.

Legacy `DKS_RESULT` schema-1 records are not converted at the trust boundary.
`dks-native-result.sh` accepts only strict `dhnt.run/v1`, so an old consumer
searching for `"classification":"pass"` cannot silently treat the new record
as its weaker schema, and the new gate cannot accept an old record. Migration
must provide an explicit, independently verified v1 pipeline plan and rerun the
worker; there is no best-effort digest synthesis.

## Strict DKS Argo lowering

`lower-argo` compiles a portable pipeline into deterministic Argo Workflow
YAML without submitting it. The required `dhnt.argo-binding/v1` sidecar keeps
cluster-only facts out of the portable contract:

```json
{
  "schema": "dhnt.argo-binding/v1",
  "workspace": {
    "claimName": "nanochat-workspace",
    "mountPath": "/workspace"
  },
  "tasks": [{
    "id": "unit-test",
    "image": "registry.example/nanochat@sha256:<64 lowercase hex>",
    "artifacts": [
      {"name": "source", "path": "source"},
      {"name": "unit", "path": "artifacts/unit"}
    ],
    "timeoutSeconds": 600,
    "retryLimit": 1
  }]
}
```

The claim must already exist. Every pipeline task needs exactly one binding,
an immutable digest-pinned image, and exact path coverage for every declared
input and output artifact. Paths are relative to the mounted workspace.
Timeout and retry are optional execution policy; when present they map to
Argo `activeDeadlineSeconds` and `retryStrategy.limit`.

The compiler accepts only `cluster` and `container` tasks targeting
`k3s/linux`, with `single` distribution or an already-expanded `shardable`
task carrying an immutable chunk identity. It emits hard selectors for Linux,
the declared architecture, and `outpost.dhnt.io/backend=k3s`. It rejects the
entire pipeline on native/cloud lanes, non-k3s platforms, replicated or
topology-coupled work, multiple matrix rows per task, missing images, partial
artifact bindings, unknown fields (including secret references), or reserved
runtime environment names. It never silently filters unsupported tasks and
never turns native or topology-coupled training into an OCI job.

The generated container receives literal task environment plus
`DHNT_INPUT_<NAME>_{PATH,SHA256}` and
`DHNT_OUTPUT_<NAME>_{PATH,SHA256}` metadata. The PVC is the transport boundary:
this compiler does not copy artifact bytes, inspect secret stores, verify
checksums, or publish results atomically. A workload claiming
`dhnt.run/v1` evidence must use a separately trusted runner that verifies input
bytes before execution and atomically verifies/publishes outputs. Argo
`Succeeded` and these injected digest values alone are not release evidence.

The current `dhnt.argo-binding/v1` lowerer accepts only
`dhnt.pipeline/v1`. It rejects v2 rather than dropping artifact kinds or output
commit semantics. The next migration must introduce a trusted-runner-aware
binding that declares the output commit-manifest path and interposes that runner
without shell reinterpretation.

The separate GNU Bash 5.3 conformance jobs remain aggregated by the existing
authoritative harness. This v1 slice strengthens native three-platform evidence
without changing Bash 5.3 requirements, adding Argo, or mutating a cluster.
