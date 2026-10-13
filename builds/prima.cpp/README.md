# Prima first-party verified build

Sprint 436 / #2029 / `17ba375e3318`. This is a CI implementation and artifact
contract. **Hosted build, audit, no-egress, signing and release evidence are
pending. No artifact SHA is approved by this commit.**

## Reviewed source

`bashy-prima-0.1.0` peels to
`f4ea9845dc91a18793921492e50ef23ec799e93e` in `qiangli/prima.cpp` (remote tag
and main checked 2026-10-13). The operator admits this as first-party MIT source;
the official-upstream-release rule does not apply. The exception is restricted
in the renderer to this repository and its `bashy-prima-X.Y.Z` source tags.
All existing third-party workflows retain their official release API gate.
The assigned umbrella policy worker records this decision in
`docs/bashy-service-supply-chain-policy.md` (worker `48d53363f`, integrated umbrella `ca6bf0f75`).

Compared with `1ea4707ea41dd4c89088c9c045d0049907f1d201`, the source includes
`ca4b56ad` (MIT nng transport shim, atomic multipart forwarding, timeout tests,
fio failure fallback) and `f4ea9845` (parallel Make dependency ordering).
The six-file diff was reviewed; the old `shard-binaries` asset is not used.
The root MIT license SHA-256 is
`0ed0eea0d0f31b7f07774a7cac64f9fd20351faf51fab447e9444734448d0452`.

The CMake overlay supplies the nng, profiler and network-utils wiring missing
from the source CMake target. nng v1.9.0 is fetched as source and checked against
`29aff2bacc9e2cfa6dcc52155d2816582617f918` and its license blob. nng links
statically with TLS and HTTP off. `prepare.py` removes fio execution entirely,
using fixed 0.1 GB/s disk estimates. HiGHS is excluded: this product uses the
source's non-HiGHS scheduling fallback or an explicit layer split, not the
optional HiGHS optimizer. Model downloads, RPC, OpenMP, BLAS and non-Metal GPU
backends are disabled. The product is `llama-cli`, not `llama-server` or llava.

## Pipeline

| Raw asset | Standard hosted runner | Backend |
|---|---|---|
| `prima-linux-amd64` | `ubuntu-24.04` | CPU |
| `prima-linux-arm64` | `ubuntu-24.04-arm` | CPU |
| `prima-darwin-arm64` | `macos-15` | Metal compiled and embedded, CPU smoke |
| `prima-darwin-amd64` | `macos-15-intel` | CPU |

These are standard runners, free for public repositories per
[GitHub's runner reference](https://docs.github.com/en/actions/reference/runners/github-hosted-runners).
No Dragon build or container is required. The Apple Silicon job must compile
Metal; the hosted runtime test requests CPU execution, so it does not attest
GPU execution on consumer hardware. Windows is unsupported and must fail closed.

Each native build uploads its compiler dependency inputs, SHA-256 inventory,
linkage report and CMake cache. The audit requires all four input sets, verifies
archive contents against their inventories, and scans the combined tree with
ScanCode 32.5.0. It rejects scanner errors and unreviewed license expressions.
CycloneDX records reviewed packages and every input file hash. OSV checks the
source/nng commits and vendored JSON version; Grype 0.120.1 checks the SBOM and
fails on high/critical advisories. There are no vulnerability waivers. Reports,
license texts/notices and compiler inputs accompany the signed release.

`dependencies.json` records included, excluded and host-system dependencies.
Compiler and OS runtime libraries are reported by `ldd`/`otool`; they are not
bundled. The scanners cannot prove the absence of vulnerabilities in a C++
fork or infer all vendored ancestry. Database evidence is time-specific; a
zero scanner finding must not be described as a complete security proof.

The no-egress test generates a deterministic synthetic GGUF (no model download),
runs single-node inference, the nng multipart/timeout test, and two-rank
inference with a `1,1` layer split. Every subprocess has a deadline. Linux runs
unprivileged inside a loopback-only network namespace with outbound syscall
tracing, including failed attempts. Darwin uses a sandbox allowing only explicit
loopback ring ports. Both allowlists carry exactly seven loopback ports: the
three ring-test ports from pinned `common/zmq_nng_shim_test.cpp` — 5599
(multipart), 5600 (rcvtimeo), 5601 (single-message) — plus the rank-offset
engine ports 19000/19001 (data) and 19100/19101 (signal). Port 5601 was
missing until candidate -3 run 38414127942, where its denied connect hung the
Mac ring test and tripped the Linux verdict; non-loopback destinations remain
denied on both platforms. Enforcement is proven by a positive control whose EPERM
denial is observed directly in the attempt's own stderr; the unified log is
not a reliable sandbox observer (hosted macos-15 run 38411861522 recorded no
denial there), so its absence neither passes nor fails the gate, while any
observed engine denial — EPERM in the engine log or a unified-log match —
fails the job. Engine or ring failure under the deny profile also fails the
job. Every Linux failure path reprints the buffered `*-egress.txt` evidence
to the step log before exiting, and the workflow uploads that file with
`if: always()`, so a gate failure always leaves its cause in the hosted log
and in the per-leg artifact (Linux legs of the same run left neither).
Unsigned build outputs still upload on success only, and signing still waits
on every gate. This is bounded scenario evidence, not all-path coverage.

Only after pin, all four builds/no-egress tests and the audit pass does the
release job sign every file and SHA256SUMS keylessly, self-verify the exact
identity/issuer, attest checksums and publish. Neither the source tag nor these
local tests are signing evidence.

## Consumer contract

First candidate release tag: `bashy-bashy-prima-0.1.0-1`. The doubled `bashy`
preserves the shared `bashy-<source-tag>-N` convention. The source tag and
artifact tag are distinct; the latter points to the reviewed generated
`bashy-build` tree, not the source commit.

For a reviewed release tag `T`:

- Repository: `qiangli/prima.cpp`; files are the four raw executables above.
- Exact certificate identity:
  `https://github.com/qiangli/prima.cpp/.github/workflows/bashy-build.yml@refs/tags/T`.
- Exact issuer: `https://token.actions.githubusercontent.com`.
- Verify both `SHA256SUMS.sigstore.json` against `SHA256SUMS` and
  `ASSET.sigstore.json` against `ASSET` using `cosign verify-blob`.
- Reject duplicate/malformed checksum rows. Require asset bytes, signed
  checksum row and independently reviewed consumer SHA-256 pin to agree.
- No identity regex, mutable latest lookup, legacy cache, upstream/PATH fallback
  or consumer-side ad-hoc codesign. Verification must precede execution.

`files/verify-asset.py DIRECTORY T ASSET EXPECTED_SHA` is an executable reference
contract, not a downloader. Consumer integration is owned separately by the
conductor's Yoke/Outpost workers. Real signatures and SHA pins must be supplied
after successful hosted CI; consumer provisioning remains unavailable meanwhile.

## Validation and handoff

The resumed worker ran `builds/check.sh` across all eleven declarations
(reproduction, structural gates, shell syntax, shellcheck and actionlint), the
11 shared egress verdict cases, 19 Go toolchain gate cases, and seven Python
contract/archive tests. Source pin verification is read-only. These checks do
not run CMake, an engine or a container.

The repository default gate builds Bashy and runs full Go and Bash suites;
that heavy gate is deferred to a claimed remote host or hosted CI under the
operator's Dragon policy. The worker's explicit gate is
`bashy gate --command 'bash builds/check.sh && python3 -B builds/prima.cpp/check_test.py'`.

Conductor next steps: review/install `fork/` in the source repository's
`bashy-build` branch, push the distinct artifact tag, inspect every job and
published bundle, independently verify/download and commit consumer SHA pins,
then run Outpost's two-host proof. The worker does not push or publish.

## Candidate -4 two-rank shutdown failure

Authenticated run `38416093564` reaches rank-0 generation and evaluation on
Linux amd64 and Darwin arm64, then times out waiting for rank 1. Source review
found unchecked nonblocking STOP sends after asynchronous nng dial, plus an
unsynchronized shared stop pointer. See the [diagnosis and required source
repair](../../docs/plan-prima-two-rank-timeout.md) for exact sites, evidence
hashes, inference limits and validation. The first-party repair is not applied
as a producer overlay; the source pin and hosted gate remain red.

The producer now reports per-rank configured endpoints, PID, pre-cleanup exit
status, warmup/evaluation markers, per-rank log-limit evidence
(log_limit_exceeded, observed stdout bytes) and cleanup status. One 90-second
deadline covers both ranks; successful master evaluation alone never passes.
Each rank's piped stdout is drained by a bounded pump that keeps the first
1 MiB per log; excess kills the rank and fails the stage with an explicit
log-limit error. The engine process itself runs with no file-size limit, so
its own files (Metal cache, model writes) are unconstrained — candidate -5
run 38418158569 showed the previous process-wide RLIMIT_FSIZE=1MiB killing
the single-rank Darwin engine with SIGXFSZ after 2.2s. Logs display as
bounded head/tail excerpts. Linux
exercise failures retain timestamped outbound/shutdown syscall excerpts in the
egress artifact and step log; the full trace is still used for the egress
verdict when the exercise succeeds. Trace overflow fails closed. The generated
fork is reproduced from these files. Focused fixture tests require no native
build, model execution or container.
