# Chunked-conformance evidence run — 2026-07-18

End-to-end, privacy-scrubbed evidence for the distributed-conformance
tooling (chunk manifests, structured records, aggregation, verdict
equivalence). All measurements were made from a clean, isolated clone
pinned at commit `8dfef8f` with siblings at the `.sibling-pins` SHAs;
the aggregator defect fix (`a457631`) was applied and used as the
aggregation instrument after review. No licensed test-suite material
was involved anywhere in this run (GNU Bash 5.3's own fixture suite
only), and nothing here is a certification claim.

Host identifiers are scrubbed throughout: the native host appears only
as `native-host` (a macOS arm64 development machine), user paths as
`$WS`. The committed JSON records referenced below were produced with
`BASH53_RUN_ID` set per lane; runner names in this document are
pseudonymous.

## Lanes and results

| # | Lane | Command core | Result | Wall |
|---|------|--------------|--------|------|
| 1 | Authoritative serial | `make test-bash` (all 86 fixtures, no skips) | **86 passed / 0 failed / 0 skipped / 0 timed out**, exit 0 | ~130 s |
| 2a | Native unchunked JSON reference | `bash53suite -json -of 1 -shard 0` | 86 passed, exit 0 | 125 s |
| 2b | Native manifest chunks | `bash53suite -json -chunk i/8`, i=1..8, sequential, shared `BASH53_RUN_ID` | every chunk exit 0 | 63+18+7+8+8+10+16+12 s |
| 2c | Native aggregation | `bash53aggregate -reference <2a> <2b×8>` | **accepted, verdicts match reference**, exit 0 | <1 s |
| 3a | Container unchunked reference | same harness, linux/arm64 static build, `gcc:14-bookworm` via podman | 79 passed / 7 failed, exit 1 | 119 s |
| 3b | Container manifest chunks | `-chunk i/8` in the same container lane, `BASH53_RUNNER=container-lane` | verdict union == 3a exactly | 62+21+9+8+7+6+6+8 s |
| 3c | Container aggregation | `bash53aggregate -reference <3a> <3b×8>` | **accepted, verdicts match reference**, exit 0 | <1 s |

Negative controls (all must refuse, and did):

- 7-of-8 chunk set → `got 7 chunk records, want 8`, exit 2.
- Native + container mixed set → refused (cross run/context), exit 2.

## Equivalence statement

Within each venue, chunked execution is verdict-equivalent to unchunked
execution:

- **Native (darwin/arm64):** serial text lane, unchunked JSON lane, and
  the 8-chunk union produce the identical 86-fixture verdict set
  (all `passed`), with full coverage and no duplicates.
- **Container (linux/arm64, glibc image):** unchunked and 8-chunk union
  produce the identical 86-fixture verdict set (79 `passed`,
  7 `failed`), reproduced identically across two independent lane runs.

Cross-venue verdict sets legitimately differ and are **refused** by the
aggregator by design (execution-context identity), which is the correct
behavior: merging attestations across contexts would manufacture
evidence.

The 7 container failures are venue baseline, not chunking artifacts and
not native regressions: the fixtures `execscript, glob-test, intl,
new-exp, printf, redir, test` fail under the container's conditions —
e.g. `test` refuses to run as root (containers run as root), and `intl`
requires the `de_DE.UTF-8` locale the stock image does not carry. The
native lane remains 86/86.

## Defects found by this run (both fixed)

1. **Aggregator context identity included per-invocation timestamps**
   (`started_at`/`finished_at`), so every genuine chunk set was refused
   as cross-context; unit tests passed on synthetic byte-identical
   contexts. Fixed in `a457631`: identity is now the stable tuple
   `{runner, commit, host_os, host_arch, bash_path}`, with regressions
   for the accept case (timestamps differ) and the refuse cases
   (identity field differs, missing chunk, duplicate chunk).
2. **Container runner identity was a random container ID**
   (`os.Hostname()` inside `podman run`), so container chunk sets could
   not aggregate without manually injecting `BASH53_RUNNER`. The
   standard container leaf now derives a deterministic, non-sensitive
   runner name; see the follow-up issue and its regression tests.

## Vocabulary note

The JSON records emit lowercase verdict values (`passed`, `failed`,
`skipped`, `timed_out`). An early design sketch showed uppercase
(`PASS`); the committed records are canonical and the sketch is
superseded — no wire change was made, deliberately, because records
already exist on disk in the lowercase vocabulary.

## Reproduction

```sh
# isolated clone at the pinned commit, siblings per .sibling-pins
make build-bash
make test-bash                                   # authoritative serial gate
go build ./tools/bash53suite ./tools/bash53aggregate
BASH53_RUN_ID=<id> bash53suite -json -of 1 -shard 0 > ref.json
for i in 1 2 3 4 5 6 7 8; do
  BASH53_RUN_ID=<id> bash53suite -json -chunk $i/8 > chunk-$i.json
done
bash53aggregate -reference ref.json chunk-*.json # exit 0, verdicts match
```

Container lane: identical, with the harness and testee cross-built
`GOOS=linux CGO_ENABLED=0` and executed inside `gcc:14-bookworm` with
the repo and fixture tree bind-mounted (see `dag.md`
`test-bash-container-prepare` / `test-bash-container-run`).
