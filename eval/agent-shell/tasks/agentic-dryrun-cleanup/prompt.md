You are running a shell-task benchmark under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell wrapper: `__EVAL_SHELL__`

Use the required shell wrapper for shell commands so the benchmark can log shell
usage. Do not invoke `bash` or `sh` directly.

Task:

Patch `scripts/prune-cache.sh` so it removes generated cache files but preserves
checked-in fixtures and metadata.

Portability:

- Do not use `find -delete`; bashy-current's pure-Go coreutils does not support
  that operation.
- After editing, the task-local check is:
  `__EVAL_SHELL__ --script /workspace/.benchmark/agent-check.sh`

Acceptance:

- Running `./scripts/prune-cache.sh` succeeds.
- `fixtures/golden.txt` still exists.
- `cache/generated.bin` is removed.
- In the `bashy-current` environment, use bashy's dry-run mode before executing
  the destructive cleanup for real. In `gnu-bash53`, inspect manually and make
  the same safe edit.
