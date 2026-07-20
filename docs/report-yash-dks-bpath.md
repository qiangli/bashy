# yash POSIX suite on DKS via the B-path — result report

**Issue #24 · run 2026-07-20 · conductor: agent-weave-issue-24**

The bash-5.3 B-path (native k8s Indexed Job on DKS, 86/0) was mirrored for the
yash POSIX (`-p`) suite. **The gate passed: the DKS chunked aggregate equals the
single-host serial run, both `50 passed / 0 failed / 0 skipped / 0 timed out`.**

## The GATE — chunked == serial (verified by running it)

| run | how | Results |
|---|---|---|
| **serial** | `bashy podman run` the image, no `CHUNK` (full 50-fixture corpus, one process) | `50 passed, 0 failed, 0 skipped, 0 timed out` |
| **DKS chunked** | 8-pod Indexed Job on `dog`, aggregated from per-pod logs | `50 passed, 0 failed, 0 skipped, 0 timed out` |

Per-chunk (identical local-podman and on-cluster): chunk 1=7, 2=7, 3..8=6 → Σ=50.
All 8 pod logs were present; `scripts/k8s-job-aggregate.sh` exited 0 (it refuses a
total if any pod log is missing — a missing log is an evidence gap, not a pass).

Evidence (on-cluster, read from `outpost kubectl logs`):

```
yash-conformance-0  chunk 1/8  Results: 7 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-1  chunk 2/8  Results: 7 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-2  chunk 3/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-3  chunk 4/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-4  chunk 5/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-5  chunk 6/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-6  chunk 7/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
yash-conformance-7  chunk 8/8  Results: 6 passed, 0 failed, 0 skipped, 0 timed out
----
AGGREGATE (8 chunks): 50 passed, 0 failed, 0 skipped, 0 timed out
```

Job: `completedIndexes: 0-7`, `succeeded: 8`, all pods on
`novidesign-4c7adeb6` (dog). Namespace `user-5c6b0a233188`. Wall clock ~20 s.

## What "50 passed" means — read this before quoting the number

`tools/yashsuite` reports a fixture **passed** iff yash's `run-test.sh` completes
without a *critical* harness error. By run-test.sh's own design it returns 0 even
when individual POSIX cases fail ("Failure of test cases does not cause the script
to return non-zero"). So this per-fixture number is a **harness-completion**
signal — "bashy ran every shell-only `-p` fixture to completion with no critical
break" — **not** the per-case conformance rate. The richer per-case rate (bashy
≈ 96 %, at bash-parity on the true delta) is what `make test-yash`
(`scripts/yash-scoreboard.sh`) reports; see `docs/yash-conformance-gap.md`. The
B-path claim proven here is the same one proven for bash-5.3: **the chunked
placement vehicle reproduces the single-host result exactly** — 8 independent pods
on a CRI runtime sum to the serial number, fixture-for-fixture.

## Corpus (the manifest was stale — regenerated)

The committed `yash-chunks.json` was unvalidated fiction (`measured_at: ""`,
`result: "not measured"`): it named 9 fixtures absent from current yash and omitted
others, so yashsuite's strict corpus↔manifest bijection could never validate — no
chunked run was possible against it. Regenerated against the real corpus:

- yash source pinned at **`7070575eec5accee71cbaa0f46accd970c9e8888`** (2026-07-07).
- **50 shell-only `-p` fixtures**, 8 chunks (round-robin). Excluded the
  job-control/signal/TTY suites (`sig*`, `bg`, `fg`, `job`, `kill*`, `wait`,
  `testtty`, `async`) — they need a controlling TTY + yash's `checkfg` helper and
  hang headlessly (the same exclusion `scripts/yash-scoreboard.sh` already uses).
- The GPL corpus is a gitignored runtime clone (`.yash-tests/`), **never committed
  and never vendored** — baked only into the throwaway runtime image.

## Artifacts produced

| file | change |
|---|---|
| `tools/yashsuite/main.go` | emit a `Results:` summary line (non-JSON path) in bash53suite's exact shape — the Job wrapper + aggregator key off `^Results:` |
| `yash-chunks.json` | regenerated: 50 real shell-only fixtures / 8 chunks; measurement recorded (serial 50/0/0/0, 2026-07-20) |
| `tools/bash53-container/Containerfile.k8s.yash` | new: bakes linux/arm64 testee + yashsuite + pruned corpus + manifest; `sigdfl` shim; `busybox`-as-`sh` harness; `ENV LANG=C` |
| `scripts/build-conformance-image.sh` | new `SUITE=yash` branch (prunes corpus to manifest, builds the yash Containerfile); `SUITE=bash53` unchanged/default |
| `docs/plan-yash-dks-bpath.md` | the plan of record |
| `docs/report-yash-dks-bpath.md` | this report |

## Two image bugs found + fixed (empirically, by running it)

1. **`LANG: parameter not set` (rc=2, every fixture).** run-test.sh dereferences
   `$LANG` under `set -u` before it exports `LANG=C`. Fix: `ENV LANG=C`.
2. **`LINENO: parameter not set` (rc=2, every fixture).** The image's `/bin/sh` is
   dash, which lacks `$LINENO`-under-`set -u`; yashsuite invokes the framework via
   PATH `sh`. `yash-scoreboard.sh` drives the harness under **busybox ash** for
   exactly this reason. Fix: symlink `/usr/local/bin/sh -> busybox` so the
   PATH-resolved harness interpreter matches the serial scoreboard's — the
   apples-to-apples the chunked==serial claim requires. (Not the `#!/bin/sh`
   shebang; only `exec.Command("sh", …)` resolution.)

Was `sigdfl` needed for yash? Baked for parity, but with LANG+busybox fixed the
suite already went 50/0 under `podman run` (which doesn't inject the CRI realtime-
signal mask). It is a cheap correctness insurance for the k8s path (trap-p), a
no-op elsewhere — kept.

## Run recipe (reproduce)

```sh
git clone --depth 1 https://github.com/magicant/yash.git .yash-tests   # GPL, gitignored
SUITE=yash ARCH=arm64 SQUASH=1 OCI="bashy podman" scripts/build-conformance-image.sh
# serial baseline:
bashy podman run --rm --platform linux/arm64 localhost/yash-conformance-k8s:latest
# side-load into dog (dragon is NoSchedule-tainted, so tolerationless pods land on dog):
bashy podman save localhost/yash-conformance-k8s:latest \
  | ssh -C novidesign.local 'bash -lc "podman exec -i novidesign-runtime k3s ctr -n k8s.io images import -"'
# fan out + aggregate:
NAME=yash-conformance NS=user-5c6b0a233188 IMAGE=localhost/yash-conformance-k8s:latest \
  ARCH=arm64 CHUNKS=8 \
  SUITE_CMD='./bin/yashsuite-linux-arm64 --tests-dir /yashtests --chunks-manifest /work/yash-chunks.json --shell ./bin/bash-linux-arm64/bash --chunk ${CHUNK}' \
  scripts/dag-to-k8s-job.sh | outpost kubectl apply -f -
NS=user-5c6b0a233188 JOB=yash-conformance KUBECTL="outpost kubectl" scripts/k8s-job-aggregate.sh
```

## Out of scope (as instructed)

The licensed **VSC-PCTS POSIX certification** is a single-SUT cert against a
non-redistributable suite; it is **not** run chunked on DKS and its content is
never baked into any image, commit, or log. The authoritative POSIX-cert run stays
single-host + unchunked. This work covered only the open **yash** suite.
`dragon`'s tainted node and cluster-admin were left untouched.
