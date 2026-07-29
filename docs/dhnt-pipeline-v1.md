# Portable pipeline and run evidence v1

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
- `tasks`: task identity, closed `lane`, dependencies, argv, repository-relative
  working directory, and literal non-secret environment;
- `matrix`: one or more exact task/platform requirements, each with the expected
  named input and output SHA-256 digests.

Lanes are exactly `native`, `container`, `cluster`, and `cloud`. Platforms use
the closed v1 backend set `local`, `vk-native`, `vk-podman`, `k3s`, and `cloud`;
OS is `linux`, `darwin`, or `windows`; architecture is `amd64` or `arm64`.
Every task must occur in the matrix. A native task must use `vk-native`.
Dependencies must exist and be acyclic.

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

## Strictness and deterministic encoding

All SHA-256 values are lowercase 64-hex. Unknown and duplicate JSON fields,
multiple JSON values, unknown enum values, duplicate task/artifact/matrix
identities, invalid timestamps, and malformed identifiers are rejected.

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

The separate GNU Bash 5.3 conformance jobs remain aggregated by the existing
authoritative harness. This v1 slice strengthens native three-platform evidence
without changing Bash 5.3 requirements, adding Argo, or mutating a cluster.
