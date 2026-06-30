# Agent Shell Evaluation Retro 2026-06-29 Pilot

## Run Summary

Pilot scope:

- Tool: `codex`.
- Environments: `bashy-agentos`, `gnu-bash53`.
- Tasks: `wrong-cwd-recovery`, `dryrun-safe-edit`.
- Valid paired task runs: 4.
- Harness shakeout rows: 2 invalid rows retained in the metrics snapshot.

All prompt-only task runs passed. After user review, these runs are classified
as **harness shakeout**, not valid bashy-vs-GNU evidence: the tools were not
container-confined, and prompt-level shell selection is not strong enough. The
next valid run must use `bashy podman` containers and must compare the `bashy`
binary, not this repo's `bin/bash` drop-in, against real GNU Bash 5.3.

## Metrics

Prompt-only shakeout runs:

| Task | Env | Success | Wall Time | Input Tokens | Output Tokens | Notes |
| --- | --- | --- | ---: | ---: | ---: | --- |
| wrong-cwd-recovery | bashy-agentos | pass | 44s | 77898 | 1069 | Adapted after `-lc` failed; bashy hints surfaced. |
| wrong-cwd-recovery | gnu-bash53 | pass | 32s | 51097 | 852 | Direct GNU Bash 5.3 path; wrapper missed shell command count. |
| dryrun-safe-edit | bashy-agentos | pass | 68s | 134054 | 3001 | Did not discover `--dryrun`; hit `find -exec` limitation. |
| dryrun-safe-edit | gnu-bash53 | pass | 44s | 78854 | 1497 | Hit missing `rg`; patched and verified cleanly. |

Raw curated rows: [`pilot-results-2026-06-29.jsonl`](pilot-results-2026-06-29.jsonl).

## Tool Findings

- `codex` completed both tasks in both environments.
- `codex` naturally uses `bash -lc` / shell `-lc` idioms. Bashy must support
  that if it wants to be frictionless for agentic tools.
- `codex` obeyed the requested shell path when it was explicit in the prompt.
- Token and command counts are not yet comparable because command logging missed
  direct shell-path invocations in one run and model prompts included different
  error recovery context.

## Bashy Product Findings

- **Implemented during pilot:** support `-l` and combined `-lc` invocation flags
  in bashy/bash drop-in flag normalization. Verified with `bin/bashy -lc 'echo hi'`,
  `bin/bash -lc 'echo hi'`, and `go test ./internal/cli`.
- **Dry-run discoverability gap:** `bin/bashy --dryrun -c ...` works, but
  `bin/bashy --help` does not advertise `--dryrun`. The agent did not discover
  dry-run and used `bashy -n` instead. Add bashy-only help surfacing without
  polluting the pure `bin/bash` drop-in help.
- **Pure-Go `find` gap:** the bashy userland emitted a useful hint but rejected
  `find -exec`, which is a common agent idiom. Either implement a safe subset,
  improve fallback guidance, or ensure agents can reach system `find` when
  needed.
- **Useful hints:** bashy emitted hints for `find` and `cd`; they were visible in
  the transcript. The `cd` hint suggested `awd`, but that is not meaningful if
  the task expects Bash portability. Consider context-sensitive hint suppression
  or clearer "bashy-only" wording.

## GNU Control Findings

- A real GNU Bash 5.3 control was not on PATH. Built a temporary control from
  `external/bash-5.3` at `/tmp/bashy-eval-gnu-bash-5.3/src/bash`.
- The restricted control PATH missed `rg`; Codex tried it once and recovered.
  Future control arms need a declared userland: either GNU Bash + GNU/coreutils
  only, or GNU Bash + normal developer PATH. Mixing both makes results harder to
  interpret.
- GNU Bash 5.3 login shells sourced a broken local `.bash_profile` reference in
  the wrong-cwd task. Future runs should use `--noprofile --norc` or equivalent
  unless startup-file behavior is under test.

## Conductor Findings

- Creating the sprint, baton, and run log early helped recovery and made the
  pilot state clear.
- The first model run should always be a harness shakeout; two invalid rows were
  caused by harness mistakes, not model behavior.
- The conductor should inspect tool event JSON, not just final verifier status.
  The product findings came from the event logs.
- The run log belongs in `docs/agent-shell-eval/`; raw generated output under
  `results/` is not sufficient for campaign memory.

## Harness Gaps

- The harness must move into `bashy podman` containers. PATH wrappers and prompt
  instructions are insufficient because agents can call `/bin/zsh` or absolute
  shell paths.
- The bashy arm must expose `bashy` as the shell under test. This repo's
  `bin/bash` is a pure GNU Bash 5.3 drop-in and should not be expected to show
  agentic-feature differences.
- The current lean `bin/bashy` cannot run `bashy podman`; it reports that
  container engines are not included. Next run needs `make build-host`, a
  `bashy_engines` build, or a mesh host with podman support.
- Command logging only wraps `bash` and `sh` from PATH. It misses direct
  invocations like `/tmp/.../bash -lc`. Add a shell proxy path that is passed to
  the prompt as the required shell, so every shell command is logged.
- Harness scripts should remain pinned to `/bin/bash` so harness behavior is not
  affected by the shell under evaluation.
- Add timeout controls around each tool invocation.
- Add automatic extraction of Codex usage fields from JSONL logs.
- Add a validity column that distinguishes harness failures from agent failures.
- Keep docs snapshots under `docs/agent-shell-eval/` after every run.

## Next Sprint Changes

- Replace the prompt-only harness with a `bashy podman` container harness.
- Build two images: `bashy-agentos-container` and `gnu-bash53-container`.
- Add `--noprofile --norc` or equivalent isolation for GNU Bash 5.3 control
  unless startup files are the task target.
- Add a `claude` paired pilot after command logging is fixed.
- Delay `opencode` and `aider` until harness validity is stronger, because they
  are DeepSeek-budget-backed.
- File/implement bashy improvements for `--dryrun` help surfacing and `find
  -exec` behavior before a larger matrix.
