# Isolated test lanes

This is the operator and agent handoff for running Bashy tests concurrently on
one physical machine. A profile or suite name is never an ownership identity.
Two agents may run Profile B, the Bash 5.3 gate, or self-tests at the same time.

## Isolation model

```
agent -> weave/worktree -> lane -> OCI container -> test unit -> result tree
```

- `bashy weave` supplies a separate source checkout and branch. Never run two
  writing agents in one checkout.
- `BASHY_TEST_LANE` names public self/Bash53/Yash containers. When omitted, the
  scripts derive a stable lane from the absolute worktree path, so different
  weave workspaces do not collide. Set it explicitly for two simultaneous runs
  from the same worktree.
- `VSC_DEV_LANE` names a licensed POSIX A/B/C/D development lane. The sibling
  harness derives a separate container and atomically reserves a Linux loop
  range. The host registry makes all reservations visible.
- An arm names one execution inside a POSIX container. Full runs use a transient
  systemd unit owned by that container, so an SSH/client disconnect does not
  terminate TCC or lose the archive step.

The OCI image store and immutable base layers may be shared. Writable source,
`/vsc`, fixtures, devices, units, temporary files, and results may not.

## Suite map

| Test | Command | Container behavior | Result/authority |
|---|---|---|---|
| Go build + unit tests | `make test-self-container` | disposable Ubuntu lane; read-only source inputs copied into private `/work` | local development gate |
| GNU Bash 5.3 fixtures | `make test-bash-container` | lane-specific baked image and runtime name; 86 fixtures, PTY, read-only root | authoritative public compatibility gate |
| Yash POSIX scoreboard | `make test-yash` | lane-specific runtime name and host scoreboard directory | local conformance differential |
| POSIX Profile A | harness `make posix-sandbox-start PROFILE=A ARM=...` | private, privileged Ubuntu fixture lane | local development only |
| POSIX Profile B | same with `PROFILE=B` | independent of A and of every other B lane | local development only |
| POSIX Profiles C/D | same with `PROFILE=C` or `D` | independent provider combinations | local development only |
| Broad matrix | `bashy dag suites.md -j8 -k` | DAG schedules public suite commands; each containerized suite still uses its lane | development matrix |

## File and DAG ownership map

An agent changing this system should start here rather than search the whole
tree:

| Surface | Implementation | Contract test / task graph |
|---|---|---|
| Lane normalization | `scripts/test-lane-id.sh` | `scripts/test-isolated-lanes.sh` |
| Self build/unit container | `scripts/test-self-container.sh`, `tools/dev-test-container/Containerfile` | `make test-self-container`; normal gate remains `make test` |
| Bash 5.3 container | `scripts/test-bash-container.sh` | `scripts/build-conformance-image.sh`, `tools/bash53-container/Containerfile.k8s`, `tools/bash53suite` |
| Fixture download/cache | `tools/bash53fixtures` | `make test-bash-fixtures` |
| Yash scoreboard | `scripts/yash-scoreboard.sh` | `make test-yash`, `tools/yashsuite` |
| Public conformance DAG | `suites.md` | `bashy dag suites.md -j8 -k`; subset names follow its `###` headings |
| Build/test DAG | `dag.md` | `bashy dag dag.md <target>`; mirrors Make targets and adds chunk/fleet lanes |
| POSIX A/B/C/D containers | sibling harness `scripts/dev-posix-sandbox.sh` | `scripts/dev-posix-sandbox-test.sh` |
| POSIX in-container prepare | sibling harness `scripts/dev-posix-prepare.sh`, `scripts/bootstrap-native-do.sh` | prepare preflight plus 117-set enumeration |
| POSIX execution/archive | sibling harness `scripts/dev-posix-run.sh`, `scripts/test-arm-native.sh`, `scripts/run-arm.sh` | `ARM_DONE`, denominator, cap, journals, timing |

Do not add a second Bash fixture runner: serial, parallel, container, DAG, and
fleet modes intentionally converge on `tools/bash53suite`. Likewise, do not
route local Podman helpers into the official native POSIX entry points.

The proprietary POSIX harness remains a sibling project; licensed bytes are
injected into a private container and never baked into an image or committed.
Local container runs never replace the complete native DO release/certification
rerun.

## Public-suite examples

Different weave workspaces need no override:

```sh
bashy weave shell <run-id>
make test-self-container
make test-bash-container
make test-yash
```

Two invocations from one checkout must state different lanes:

```sh
BASHY_TEST_LANE=agent-a make test-bash-container &
BASHY_TEST_LANE=agent-b make test-bash-container &
wait
```

Useful resource and image overrides:

```sh
BASHY_TEST_CPUS=4 BASHY_TEST_MEMORY=8g make test-self-container
BASHY_TEST_TARGET=test-isolated-lanes make test-self-container
BASHY_TEST_BASE_IMAGE='docker.io/library/ubuntu@sha256:<digest>' make test-self-container
BASH53_BASE_IMAGE='docker.io/library/ubuntu@sha256:<digest>' make test-bash-container
```

Changing distributions is an adapter change, not only a tag change. Pair a new
base with `BASHY_TEST_CONTAINERFILE`; its packages, shell, locales, accounts,
and service model must satisfy the same tests. The checked-in default is the
digest-pinned Ubuntu 24.04 substrate used by the development POSIX lanes.

## Test groups

`go test ./...` is split into six named groups so lanes can run in parallel and
skip unchanged work. `scripts/test-groups.txt` is the manifest; the spec is the
"Test groups" table of story `45786df1114b`.

| Group | Contents | Cache |
|---|---|---|
| `core` | shipped libraries (`go list -deps ./cmd/bashy ./cmd/bash ./cmd/sh`) and small internal/pkg libraries | reuse |
| `agentos` | `internal/agentos` | reuse |
| `frontend` | `internal/cli`, `cmd/bashy`, `cmd/bash`, `cmd/sh` | reuse |
| `tooling` | `tools/*` except elfaudit/productgo, `scripts`, `native` | reuse |
| `examples` | `examples/*` in the root module | reuse |
| `e2e` | `tools/elfaudit`, `tools/productgo`, `cmd/bashy` binary-building tests | always fresh (`-count=1`) |

```sh
make test-groups-check                 # union == go list ./... (linux+darwin+windows), one group each
scripts/test-groups.sh --list          # group -> packages
make test-group G="core agentos"       # parallel local lanes
scripts/test-groups.sh --dry-run run e2e
```

`--check` fails closed on a package with no group, a package in two groups, an
unknown package or group, or a split-out test name that does not exist. A
package that mixes unit tests with binary-building tests keeps its home group;
the manifest names those tests on an `e2e` line, which runs them with `-run`
while the home group runs the package with the matching `-skip`. A manifest
entry whose directory has its own `go.mod` is a nested module and runs as
`go -C <dir> test ./...`, so packages can move out of the root module without
breaking the check.

Each run writes `bin/test-groups/<group>.log` and `<group>.tsv` (package,
`ok`/`FAIL`/`notests`, seconds, label). The label is `reuse` when `go test`
printed `(cached)`: the result was replayed from Go's content-keyed test cache
because the package, its dependencies, flags, environment and opened files are
unchanged. `fresh` means the test binary actually ran. Evidence for a candidate
should report both counts; `e2e` rows are always `fresh`.
`BASHY_TEST_GROUP_TIMEOUT` (default `45m`) is the `go test -timeout`.

Reuse needs a persistent cache. In the container lane:

```sh
BASHY_TEST_KEEP_CACHE=1 BASHY_TEST_GROUPS="agentos frontend" make test-self-container
```

`BASHY_TEST_KEEP_CACHE=1` keeps the volume `bashy-self-cache-<lane>-<go.sum
cksum>` (modules, build and test cache) instead of deleting it on exit; a
`go.sum` change starts a new volume, and stale ones are removed by their owner.
`BASHY_TEST_GROUPS` runs the named groups instead of `make $BASHY_TEST_TARGET`
and prints the per-group TSV. Without either variable the lane behaves as
before.

Sharding across the test hosts: give each host a fixed set of groups and a
stable lane per shard so its kept cache stays warm, for example

```sh
# the second (heavy) test host
BASHY_TEST_LANE=g-agentos  BASHY_TEST_KEEP_CACHE=1 BASHY_TEST_GROUPS=agentos BASHY_TEST_CPUS=4 make test-self-container
# the Linux gate VM on the Windows test host
BASHY_TEST_LANE=g-examples BASHY_TEST_KEEP_CACHE=1 BASHY_TEST_GROUPS=examples make test-self-container
BASHY_TEST_LANE=g-rest     BASHY_TEST_KEEP_CACHE=1 BASHY_TEST_GROUPS="core frontend tooling" make test-self-container
BASHY_TEST_LANE=g-e2e      BASHY_TEST_GROUPS=e2e make test-self-container
```

Keep `agentos` and `examples` (the two longest groups) on different hosts, and
run `make test-bash-container` (the 86-fixture gate, outside these groups)
beside `e2e`. Each host claims itself with `bashy claim host:<role>` first.
The union of all shards must be all six groups; `make test-groups-check` proves
the groups cover `go list ./...`.

## POSIX A/B/C/D examples

In the sibling `vsc-pcts-harness-kit` checkout:

```sh
VSC_DEV_LANE=agent-a VSC_DEV_CPUSET=0,1 make posix-sandbox-prepare
VSC_DEV_LANE=agent-a make posix-sandbox-start PROFILE=B ARM=agent-a-b-r1

VSC_DEV_LANE=agent-b VSC_DEV_CPUSET=2,3 make posix-sandbox-prepare
VSC_DEV_LANE=agent-b make posix-sandbox-start PROFILE=B ARM=agent-b-b-r1

make posix-sandbox-lanes
VSC_DEV_LANE=agent-a make posix-sandbox-status
VSC_DEV_LANE=agent-a ./scripts/dev-posix-sandbox.sh logs agent-a-b-r1
```

The registry serializes loop allocation. A caller may explicitly set
`VSC_DEV_LOOP_BASE`, but a range already reserved by another lane is rejected;
the in-container bootstrap independently verifies actual loop ownership. CPU
sets may overlap for throughput or be disjoint for stable timing.

## Capacity planning

Isolation removes correctness collisions, not resource limits. Start with two
CPUs and four GiB per ordinary lane. The licensed POSIX image also needs four
loop devices and substantial sparse-disk headroom. Before fan-out, inspect:

```sh
bashy podman stats --no-stream
df -h
make posix-sandbox-lanes       # in the harness checkout
```

If memory pressure begins, reduce concurrent lanes; do not weaken per-test
timeouts, denominators, evidence checks, or fixture isolation.

## Status, failure, and cleanup

- Public containers carry `io.dhnt.test.lane=<lane>` labels.
- POSIX containers carry `io.dhnt.vsc.lane=<lane>` labels, and their units are
  shown by `posix-sandbox-status`.
- A reused lane/container/unit name fails closed. Choose a new lane or clean up
  the completed owner; never delete another agent's live container.
- Preserve failed evidence until reviewed. After merge/evidence retention,
  remove the agent's weave/worktree and merged branch, remove disposable public
  containers/images if no longer useful, and run
  `VSC_DEV_LANE=<lane> make posix-sandbox-destroy` for private POSIX lanes. That
  command destroys the injected licensed corpus and releases its registry slot.

The cleanup rule is ownership-based: clean what your lane created after its
result is retained, and never bulk-delete shared caches, unrelated branches,
other agents' lanes, or active evidence.
