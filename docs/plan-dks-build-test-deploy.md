# DKS build, test, and deploy plan

Status: proposed
Scope: replace bashy's host-specific SSH fanout and standing per-host QA pollers
with DKS-scheduled work while preserving the existing conformance and
byte-promotion evidence contracts.

## Findings

### Existing Bash 5.3 gate

- `tools/bash53suite` is the only authoritative runner.
- `make test-bash-parallel` / `scripts/ci-bash53-gate.sh` run all 86 GNU Bash
  5.3 fixtures, reject skips and incomplete output, and enforce the known-failure
  ratchet.
- `test-bash-chunk` and the committed `chunks.json` provide stable supplementary
  fanout. The authoritative release gate remains one complete, unchunked run.
- The current remote path (`dag-fanout`, `test-bash-chunks-fleet`, and
  `scripts/fleet-conformance.sh`) is SSH-, checkout-, and host-path-dependent.
- The container path is closer to a portable worker, but currently uses a mutable
  `localhost/bash53-conformance:latest` image and bind-mounted repository/test
  trees.

### Existing POSIX lanes

- `yash` / `yash-chunk` run the shell-only yash POSIX tests in Linux oracle
  containers and publish a scoreboard; this is an INFO metric, not the Bash 5.3
  release gate.
- `parity`, `xcu-diff`, `oils-diff`, `multishell`, and `austin` are independent
  DAG targets with different dependencies and portability constraints.
- The yash harness clones its GPL test corpus at run time and builds the Linux
  testee for the container architecture. Job-control and signal cases requiring
  a controlling TTY are deliberately excluded.
- Licensed/private suites such as VSC-PCTS must use a private namespace, private
  artifacts, and restricted retention. Their inputs or detailed results must not
  enter public images, logs, or repository artifacts.

### Existing build and release path

- GitHub CI natively builds and tests on Linux, macOS, and Windows, then
  cross-builds the six release targets.
- A `vX.Y.Z-dev` tag builds both `bashy` and `bash` archives with GoReleaser.
- Host QA downloads and checksum-verifies the exact candidate bytes, then authors
  `refs/qa/<version>/<os>`.
- The bare `vX.Y.Z` promotion byte-copies the tested prerelease assets and calls
  the cloudbox release webhook. Cloudbox then owns canary-to-fleet rollout.
- Bashy's current `dag qa` cannot pass on Windows because ZIP extraction is
  explicitly unimplemented.

### DKS substrate

- `vk-podman` is a Linux-container venue on every host. It is suitable for
  normalized conformance and high-throughput chunk fanout, but it does not prove
  native macOS or Windows behavior.
- `vk-native` executes `Command` and `Args` as a detached process on the host,
  resolving the executable from the host `PATH`. It does not materialize a
  container image or source workspace.
- `vk-native` records terminal status and a host-side log file, but its provider
  does not expose Kubernetes container logs or exec/attach.
- The podman translator supports one container, literal environment values, and
  hostPath/emptyDir abstractions. ConfigMap/Secret `envFrom`, projected data
  volumes, init containers, and sidecars are outside the current surface.
- Ready DKS capacity currently consists of two darwin/arm64 native nodes
  (`dragon`, `novidesign`) plus their podman nodes and Linux k3s agent nodes.
  There is no Ready native Windows or native Linux worker. The `puppy` agent node
  is Unknown and is not a `vk-native` node.

## Execution model

Use two explicit lanes. Never infer native platform coverage from a Linux
container running on that host.

| Lane | DKS target | Purpose |
|---|---|---|
| `container-normalized` | `outpost.dhnt.io/backend=vk-podman` | GNU Bash chunks, yash, and other container-compatible POSIX suites |
| `native-platform` | `outpost.dhnt.io/backend=vk-native` plus OS/arch labels | Native Go build/test, command dispatch, release-asset smoke, and OS-specific behavior |

The k3s agent nodes may run Linux-only control/aggregation work, but they do not
count as proof of their registered host's operating system.

## Work plan

### 1. Define one DKS job and evidence contract

Add a small bashy-owned DKS runner/controller rather than teaching every test
script Kubernetes:

- Input `TaskSpec`: run ID, source/release identity, lane, target, chunk, required
  capabilities, time/resource limits, and artifact policy.
- Output `RunRecord`: start/end, selected node and actual OS/arch/backend, exit
  status, result classification (`pass`, `conformance-fail`, `infra-fail`,
  `incomplete`), checksums, and artifact references.
- Require one terminal record for every planned task. Missing records and missing
  result summaries fail closed.
- Select nodes with the DKS backend, OS, arch, and registered-host labels; retain
  the virtual-kubelet toleration in generated Pods/Jobs.
- Put GitHub write credentials only in a trusted aggregator. Workers download
  public candidates or scoped private inputs and return evidence; they never
  author promotion refs.

Because virtual-node logs are not currently available through `kubectl logs`,
workers must upload a bounded result bundle to an authenticated artifact endpoint.
Adding Kubernetes log support in outpost is still desirable for diagnostics, but
it must not be the sole evidence channel.

### 2. Make workers self-contained

Do not depend on a pre-existing checkout or user-specific path.

- Produce a content-addressed source bundle once per commit for native jobs.
- Produce digest-pinned OCI worker images for normalized jobs. Include the
  compiled Linux testee, `tools/bash53suite`, helper-build prerequisites, and a
  verified Bash 5.3 corpus snapshot or fetch manifest.
- Stage the native bundle with a small installed `bashy-dks-worker` executable.
  The worker downloads, checksum-verifies, expands into its per-run directory,
  runs the requested target, uploads evidence, and removes the workspace.
- Use direct literal environment values for non-secret task metadata. Give the
  worker a short-lived artifact credential through an outpost-mediated mechanism;
  do not expand `vk-native` into a general host-volume or Kubernetes-Secret
  implementation merely for this feature.
- Pin corpora, images, and source by digest/SHA. Remove the mutable `latest` image
  and host bind mounts from the canonical DKS path.

### 3. Move conformance onto `vk-podman`

- First run one GNU Bash chunk on one DKS podman node and validate result upload,
  timeout handling, node loss, retry classification, and no host-artifact
  leakage.
- Fan out the committed Bash chunks over all eligible podman nodes. Chunk
  membership remains independent of available fleet size.
- Run the complete unchunked `tools/bash53suite` as a separate DKS Job and keep
  it as the canonical 86-fixture gate. The chunked campaign is throughput and
  diagnostics, not a substitute for the canonical gate.
- Move `yash-list`/`yash-chunk` to the same mechanism, preserving its INFO
  semantics and merged scoreboard.
- Migrate `parity`, `xcu-diff`, `oils-diff`, `multishell`, and `austin` one at a
  time after declaring each target's image, network, TTY, and artifact contract.
- Keep TTY-only or licensed suites in dedicated capability/private lanes rather
  than silently skipping them.

### 4. Move native platform verification onto `vk-native`

For each supported OS, schedule on a real native node:

- build `cmd/bash` and `cmd/bashy`;
- run the existing OS-appropriate Go test selection;
- run `TestE2EAllListedCommandsDispatch`;
- run `--version`, `-c`, and embedded-tool smokes;
- record the actual node OS/arch and binary hashes in the RunRecord.

Run the six-target `CGO_ENABLED=0` cross-build as an additional build-integrity
job. Cross-build success does not replace at least one real execution on each of
Linux, macOS, and Windows.

Before this gate can be complete, register and keep Ready at least one native
Linux host and one native Windows host in addition to the current macOS nodes.
Architecture-complete native QA requires one compatible worker per released
OS/arch; otherwise declare native execution per OS plus cross-build coverage for
the unrepresented architectures.

### 5. Put candidate release QA on DKS

- Keep `vX.Y.Z-dev` and GoReleaser as the candidate build.
- Have the DKS controller create one native QA task per required OS/arch.
- Each task downloads its exact archive and `checksums.txt`, verifies SHA-256,
  extracts it, and runs the existing runtime smokes.
- Implement pure-bashy ZIP extraction (or ship a small pure-Go extractor) so the
  Windows archive can pass the same fail-closed `dag qa` contract.
- The trusted aggregator validates all RunRecords, then creates
  `refs/qa/<version>/<os>` for complete OS lanes.
- The existing bare-tag promotion and cloudbox webhook remain the deployment
  boundary. DKS validates and attests; it does not bypass release promotion or
  call infrastructure providers directly.

### 6. Cut over cleanly

Once DKS reproduces the canonical Bash result, POSIX scoreboard, three native-OS
smokes, and one complete candidate promotion:

- delete `dag-fanout`, hard-coded fleet preparation/check targets, and the SSH
  fleet script/config;
- delete the standing host and broker QA pollers replaced by the DKS aggregator;
- make the DKS build/test matrix the required pre-tag gate;
- require all three OS refs for promotion rather than the current Windows-only
  default;
- retain only local single-machine targets needed for developer iteration and
  the existing release/webhook rollout path.

## Delivery slices

1. **DKS primitive:** worker bundle, RunRecord upload, controller, one podman Job,
   one current macOS native Job.
2. **Conformance:** Bash chunk fanout plus the unchunked 86-fixture gate; yash
   fanout and aggregation.
3. **Platform matrix:** native Linux/macOS/Windows hosts, native build/test/smoke,
   six-target cross-build, Windows ZIP QA.
4. **Release integration:** DKS candidate QA, trusted ref authoring, required
   three-OS promotion, webhook rollout verification.
5. **Removal:** delete SSH/path-dependent fanout and decentralized pollers after
   one successful end-to-end release.

## Acceptance criteria

- A single command/run ID covers source build, GNU Bash 5.3, declared POSIX
  suites, native platform verification, candidate QA, and promotion readiness.
- The complete Bash 5.3 run reports all 86 fixtures with no skips or missing
  result.
- Every required task has a signed or authenticated RunRecord tied to source,
  corpus/image, node, OS/arch, and output hashes.
- Published candidate bytes execute on native Linux, macOS, and Windows before
  promotion.
- Promotion reuses the tested bytes, and the existing webhook performs
  canary-to-fleet deployment.
- Node loss, timeout, missing artifacts, absent platform capacity, and private
  suite leakage all fail closed and are distinguishable from shell conformance
  failures.
