# yash POSIX suite on DKS via the B-path (native k8s Job)

**Issue #24.** Mirror the proven bash-5.3 B-path (86/0 on DKS) for the yash
POSIX (`-p`) suite: bake a self-contained squashed image, side-load it into the
`dog` node's containerd, fan the chunks out as an Indexed Job, and prove
**chunked == serial** by aggregating the per-pod logs.

## What was reused (unchanged framework)

- `scripts/dag-to-k8s-job.sh` — emits the Indexed Job (one chunk per pod; a TEST
  failure is a pod SUCCESS read from logs, only a can't-run exits non-zero).
- `scripts/k8s-job-aggregate.sh` — sums per-pod `Results:` lines; refuses a total
  if any pod log is missing.
- `tools/yashsuite` — the chunkable Go runner (the yash analogue of
  `tools/bash53suite`); `yash-chunks.json` is its stable chunk manifest.

## What was added / changed

1. **`tools/yashsuite/main.go`** — emit a `Results: N passed, M failed, S skipped,
   T timed out` line (non-JSON path) in the exact shape `tools/bash53suite` uses.
   The Job wrapper keys pod-success off `^Results:` and the aggregator sums those
   lines; without it every yash pod would look like an infra failure.

2. **`yash-chunks.json`** — regenerated against the real corpus. The committed
   manifest was unvalidated fiction (`measured_at: ""`, `result: "not measured"`):
   it referenced 9 fixtures absent from the current yash source and omitted others,
   so yashsuite's strict corpus↔manifest bijection could never validate. The new
   manifest is the **50 shell-only `-p` fixtures** actually present at the pinned
   yash SHA, split into 8 chunks. Job-control / signal / TTY suites (`sig*`, `bg`,
   `fg`, `job`, `kill*`, `wait`, `testtty`, `async`) are excluded — they need a
   controlling TTY + yash's `checkfg` helper and hang headlessly (the same
   exclusion `scripts/yash-scoreboard.sh` already applies).

3. **`tools/bash53-container/Containerfile.k8s.yash`** — the yash sibling of
   `Containerfile.k8s`: bakes the linux/arm64 bashy `bash` testee, `yashsuite`,
   the pruned yash `tests/` corpus (run-test.sh + helpers + exactly the 50
   fixtures), and `yash-chunks.json`. Carries the `sigdfl` shim (same rationale as
   bash-5.3) and `ENV LANG=C` (run-test.sh dereferences `$LANG` under `set -u`
   before exporting it — without it every fixture dies with a critical
   `LANG: parameter not set`, rc=2).

4. **`scripts/build-conformance-image.sh`** — added a `SUITE=yash` branch that
   stages the yash payload, prunes the corpus to the manifest's fixture set, and
   builds `Containerfile.k8s.yash`. `SUITE=bash53` is unchanged (default).

## The GATE

`tools/yashsuite` reports a fixture as **passed** iff yash's `run-test.sh`
completes without a *critical* harness error (run-test.sh returns 0 even when
individual POSIX cases fail — by its own design). So this per-fixture number
measures "the harness ran the fixture to completion," a coarser signal than the
per-case conformance rate that `make test-yash` (`yash-scoreboard.sh`) reports.
The B-path claim is exactly the bash-5.3 one: **the sum over the 8 chunked pods
equals the single-host serial run of the same runner+image.** Both numbers are
reported with evidence in `docs/report-yash-dks-bpath.md`.

## Run recipe

```sh
# 0. corpus (GPL, gitignored — never committed)
git clone --depth 1 https://github.com/magicant/yash.git .yash-tests

# 1. build the squashed arm64 image
SUITE=yash ARCH=arm64 SQUASH=1 OCI="bashy podman" scripts/build-conformance-image.sh

# 2. serial baseline (full corpus, no chunk) inside the image
bashy podman save localhost/yash-conformance-k8s:latest ... # or run directly:
bashy podman run --rm --platform linux/arm64 localhost/yash-conformance-k8s:latest

# 3. side-load into dog's containerd
bashy podman save localhost/yash-conformance-k8s:latest \
  | ssh novidesign.local 'podman exec -i novidesign-runtime k3s ctr -n k8s.io images import -'

# 4. emit + apply the Indexed Job (8 chunks; pods land on dog — dragon is tainted)
NAME=yash-conformance NS=user-5c6b0a233188 \
  IMAGE=localhost/yash-conformance-k8s:latest ARCH=arm64 CHUNKS=8 \
  SUITE_CMD='./bin/yashsuite-linux-arm64 --tests-dir /yashtests --chunks-manifest /work/yash-chunks.json --shell ./bin/bash-linux-arm64/bash --chunk ${CHUNK}' \
  scripts/dag-to-k8s-job.sh | outpost kubectl apply -f -

# 5. aggregate + gate
NS=user-5c6b0a233188 JOB=yash-conformance scripts/k8s-job-aggregate.sh
```
