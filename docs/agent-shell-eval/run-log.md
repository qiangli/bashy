# Agent Shell Evaluation Run Log

## 2026-06-29 Pilot Setup

- Created sprint `#1`: `Agent shell evaluation pilot`.
- Created weave issue `#1`: pilot harness scaffold.
- Created weave issue `#2`: `codex` wrong-cwd paired pilot.
- Built temporary GNU Bash 5.3 control at:
  `/tmp/bashy-eval-gnu-bash-5.3/src/bash`.
- Verified preflight with:
  `GNU_BASH53=/tmp/bashy-eval-gnu-bash-5.3/src/bash ./eval/agent-shell/run-preflight.sh`.

## 2026-06-29 Harness Shakeout

Initial harness failures:

- `run-preflight.sh` used `BASH_SOURCE[0]`, which failed when invoked through
  the ambient bashy-compatible shell path. Fixed by using `$0` path discovery.
- `run-task.sh` isolated `PATH` before resolving the agent binary, so `codex`
  was not found. Fixed by resolving the tool path before PATH isolation.
- Shell wrappers used `#!/usr/bin/env bash`, which recursively found the wrapper.
  Fixed by using `/bin/bash` for harness wrappers.
- Harness scripts used `#!/usr/bin/env bash`, making the harness itself
  sensitive to PATH. Fixed by pinning harness/setup/verify scripts to `/bin/bash`.

These are harness findings, not agent capability results.

## 2026-06-29 Runs

These runs are now classified as **prompt-only harness shakeout**, not valid
container-enforced evidence for bashy-vs-GNU product claims. The next valid
evaluation must use the `bashy` binary inside a `bashy podman` container for the
bashy arm, and real GNU Bash 5.3 inside the control container.

| Run | Task | Env | Tool | Result | Notes |
| --- | --- | --- | --- | --- | --- |
| `20260629T233806Z-wrong-cwd-recovery-bashy-agentos-codex-85527` | wrong-cwd-recovery | bashy-agentos | codex | invalid | Harness failure: tool not found and wrapper recursion. |
| `20260629T233859Z-wrong-cwd-recovery-bashy-agentos-codex-88279` | wrong-cwd-recovery | bashy-agentos | codex | invalid/pass artifact | Verifier passed, but harness exited incorrectly before shebang fix. |
| `20260629T233942Z-wrong-cwd-recovery-bashy-agentos-codex-90012` | wrong-cwd-recovery | bashy-agentos | codex | pass | First clean bashy-agentos run. |
| `20260629T234048Z-wrong-cwd-recovery-gnu-bash53-codex-93552` | wrong-cwd-recovery | gnu-bash53 | codex | pass | First clean GNU Bash 5.3 control run. |
| `20260629T234137Z-dryrun-safe-edit-bashy-agentos-codex-95623` | dryrun-safe-edit | bashy-agentos | codex | pass | Passed, but agent did not discover `--dryrun`; used syntax check instead. |
| `20260629T234251Z-dryrun-safe-edit-gnu-bash53-codex-98894` | dryrun-safe-edit | gnu-bash53 | codex | pass | Passed. Hit missing `rg` once under the restricted control PATH. |

## 2026-06-29 Product Fix From Pilot

Codex naturally tried the common GNU Bash invocation `bash -lc '...'`. `bashy`
advertised `-l` in help but did not normalize short `-l` or combined `-lc` for
the Go flag parser, so `bin/bashy -lc 'echo hi'` failed with
`flag provided but not defined: -lc`.

Implemented and verified:

- `bin/bashy -lc 'echo hi'` now works.
- `bin/bash -lc 'echo hi'` now works.
- `go test ./internal/cli` passes.

## 2026-06-29 Harness Correction

User correction: the evaluation must compare the `bashy` binary against GNU Bash
5.3, not this repo's `bin/bash` drop-in. It must also force shell use through
containers, preferably `bashy podman`, because prompt-level shell instructions
are too weak.

Host check:

```text
bin/bashy podman info
```

Current result: unavailable in the lean build; `bashy podman` reports that the
container/LLM engines are not included and require `bashy_engines`/host build or
a host node in the mesh.

## 2026-06-29 Remote Preflight

Preferred remote execution should use a prepared high-capacity local-network
host, selected outside the repo via local SSH config or operator instructions.
Do not hard-code hostnames or usernames in the DAG.

Local network reachability:

```text
local-network remote host reachable
```

SSH preflight:

```text
initial SSH alias attempt failed
correct local SSH target succeeded
```

Status: remote command execution works via the operator-provided local SSH
target. Prepared `eval/agent-shell/DAG.md` with `preflight-remote`:

```sh
bin/bashy dag --mesh eval/agent-shell/DAG.md preflight-remote
```

DAG mesh preflight result:

```text
remote command execution succeeded
bashy=missing
podman=
dag: 1 target(s) ok
```

Status: remote execution works. Container harness is blocked until `bashy`
with podman engine support and/or podman-compatible runtime is available on
the selected remote host.

## 2026-06-29 Next One-Run Estimate

Estimate:

- tools: `codex`
- envs: `bashy-agentos-container`, `gnu-bash53-container`
- tasks: `wrong-cwd-recovery`
- runs: 2
- wall time: 2-8 minutes total on a prepared high-capacity remote host after container images exist
- token range: 40k-180k input, 1k-8k output per run
- paid budget exposure: none beyond subscription
- approval required: no for `codex`; yes before any `opencode`/`aider` run
- approval status: pending remote access/container preflight

Operational note: subscription-backed tools can still hit rate limits or API
errors. Future runs must include retry count and backoff duration in metrics,
and wall-clock time must include any retry wait.

## 2026-06-29 Container Preflight

Documented in
[`container-preflight-2026-06-29.md`](container-preflight-2026-06-29.md).

Result:

```text
container_preflight=pass
bashy_image=bashy-agent-shell:bashy-v0.4.0
gnu_image=bashy-agent-shell:gnu-bash53
```

This verifies the shell-arm substrate only. No agent benchmark result was
claimed.

## 2026-06-30 Container-Enforced Pilot

Documented in:

- [`results-2026-06-30-container-pilot.md`](results-2026-06-30-container-pilot.md)
- [`retro-2026-06-30-container-pilot.md`](retro-2026-06-30-container-pilot.md)

Scope:

- Shell arms: `bashy v0.4.0` vs `GNU Bash 5.3`.
- Agent tools: `codex`, `claude`, `agy`.
- Excluded pending cost approval: `opencode`, `aider`.
- Tasks: `wrong-cwd-recovery`, `dryrun-safe-edit`.
- Execution: host agent orchestration, task shell commands through
  `bin/bashy podman run` into the selected shell container.

Valid product-comparison result:

| Shell arm | Valid runs | Passes | Pass rate | Total wall time | Retries |
| --- | ---: | ---: | ---: | ---: | ---: |
| bashy v0.4.0 | 6 | 6 | 100% | 557s | 0 |
| GNU Bash 5.3 | 6 | 6 | 100% | 3879s | 1 |

Main observations:

- Both shell arms passed all valid runs.
- The aggregate bashy wall time was lower, but the largest difference came from
  an `agy` timeout/retry outlier on the GNU Bash arm.
- `agy` spent significant time discovering bashy's dry-run surface; this led to
  a local bashy improvement adding `--dry-run` as an alias for `--dryrun`.
- Claude emitted `rate_limit_event` telemetry with `status:"allowed"`; these are
  not API errors and are not counted as failures.

Harness/product fixes made locally during the sprint:

- Fixed `bashy -l` / `bashy -lc`.
- Added `bashy --dry-run` alias.
- Fixed the container verifier invocation.
- Fixed Claude and agy prompt adapters.
- Tightened retry/error matching and future token-summary recording.

No `opencode` or `aider` runs were performed.
