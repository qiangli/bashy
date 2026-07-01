# Agent Shell Eval Results: 2026-07-01 Current Baseline

This resumed baseline compared the current `bashy` AgentOS build against GNU
Bash 5.3 using the approved subscription-backed tools only: `codex`, `claude`,
and `agy`.

## Preflight

- Git baseline: `8736b4a7fbdd`
- Host/evaluated bashy version: `GNU bash, version 5.3.0(1)-bashy-eval-8736b4a7fbdd`
- GNU control version: `GNU bash, version 5.3.0(2)-release (aarch64-unknown-linux-gnu)`
- `bashy podman`: pass, using embedded podman/vfkit/gvproxy
- Images:
  - `bashy-agent-shell:bashy-current`
  - `bashy-agent-shell:gnu-bash53`

Harness fixes made before the run:

- Added the live `bashy-current` arm so new runs no longer target the old
  `bashy-v0.4.0` image.
- Added `autoconf` to the GNU Bash 5.3 control image build.
- Deleted copied object/archive artifacts before building GNU Bash in the
  control image, because the source tree may contain host-platform residue.

## Result Matrix

All 12 product-comparison runs passed verification and stayed valid.

| Task | Tool | Shell arm | Result | Wall time | Tool calls | Bash invocations | Retries | Retry sleep | API/rate-limit signals |
| --- | --- | --- | --- | ---: | ---: | ---: | ---: | ---: | ---: |
| wrong-cwd-recovery | codex | bashy-current | pass | 39s | 14 | 8 | 0 | 0s | 0 |
| wrong-cwd-recovery | codex | GNU Bash 5.3 | pass | 44s | 10 | 6 | 0 | 0s | 0 |
| wrong-cwd-recovery | claude | bashy-current | pass | 31s | 8 | 5 | 0 | 0s | 0 |
| wrong-cwd-recovery | claude | GNU Bash 5.3 | pass | 31s | 8 | 5 | 0 | 0s | 1 |
| wrong-cwd-recovery | agy | bashy-current | pass | 24s | 3 | 3 | 0 | 0s | 0 |
| wrong-cwd-recovery | agy | GNU Bash 5.3 | pass | 20s | 3 | 3 | 0 | 0s | 0 |
| dryrun-safe-edit | codex | bashy-current | pass | 59s | 18 | 10 | 0 | 0s | 0 |
| dryrun-safe-edit | codex | GNU Bash 5.3 | pass | 30s | 8 | 5 | 0 | 0s | 0 |
| dryrun-safe-edit | claude | bashy-current | pass | 43s | 12 | 4 | 0 | 0s | 0 |
| dryrun-safe-edit | claude | GNU Bash 5.3 | pass | 56s | 14 | 5 | 0 | 0s | 0 |
| dryrun-safe-edit | agy | bashy-current | pass | 27s | 6 | 6 | 0 | 0s | 0 |
| dryrun-safe-edit | agy | GNU Bash 5.3 | pass | 122s | 16 | 16 | 0 | 0s | 0 |

## Aggregates

| Shell arm | Valid runs | Passes | Pass rate | Total wall time | Median wall time | Tool calls | Bash invocations | Retries | API/rate-limit signals |
| --- | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: | ---: |
| bashy-current | 6 | 6 | 100% | 223s | 35.0s | 61 | 36 | 0 | 0 |
| GNU Bash 5.3 | 6 | 6 | 100% | 303s | 37.5s | 59 | 40 | 0 | 1 |

Token/cost availability:

- Codex native token totals:
  - bashy-current: 237,420 input, 209,664 cached input, 3,881 output, 568 reasoning output
  - GNU Bash 5.3: 124,121 input, 111,360 cached input, 1,838 output, 165 reasoning output
- Claude reported cost excerpts:
  - bashy-current: `$0.4826895`
  - GNU Bash 5.3: `$0.4932735`
- agy token usage was not parsed by the current harness.

## Interpretation

- Pass/fail still does not separate the arms on these two small tasks: both
  scored 6/6.
- bashy-current was faster in aggregate by 80s, mainly because `agy` completed
  `dryrun-safe-edit` much faster with bashy (`27s` vs `122s`).
- Codex used more tokens and tool calls on bashy-current in this batch. The
  command logs show it probed the bashy surface (`--help`, `--dryrun`) before
  converging; this is useful product feedback, not yet a negative conclusion.
- Claude was stable on both arms. The single API/rate-limit signal on the GNU
  arm did not cause a retry or failure and appears consistent with previously
  observed non-fatal Claude telemetry.
- agy showed the clearest bashy benefit on `dryrun-safe-edit`: it discovered and
  used `bashy --dryrun` quickly. On GNU Bash it repeatedly ran the cleanup script
  and even tried `bash --dryrun`, then still passed final verification.

## Raw Artifacts

- JSONL: `results/agent-shell-current-20260701.jsonl`
- Work/log root: `/Users/qiangli/tests/bashy-eval/runs-current`
- Per-run summaries: `docs/agent-shell-eval/test-20260701T*.md`

## Repository Verification

- `/bin/bash -n eval/agent-shell/run-container-task.sh eval/agent-shell/container-preflight.sh`: pass
- `go test ./...`: pass
- `go vet ./...`: pass
- `make test-bash TESTS=histexpand`: pass (`1 passed, 0 failed`)
- `make test-bash-parallel`: pass on rerun
  (`86 passed, 0 failed, 0 skipped, 0 timed out`)

The first `make test-bash-parallel` attempt reported a non-reproducible
`histexpand` failure (`85 passed, 1 failed`). The fixture passed in isolation
and the full parallel rerun passed cleanly. No compatibility regression is
currently reproducible.
