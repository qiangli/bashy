You are running a shell-task benchmark under environment `__ENV_NAME__`.

Workspace: `__WORKSPACE__`
Required shell: `__EVAL_SHELL__`

Use shell commands to inspect the cleanup workflow. Prefer invoking `bash` or
`sh` from PATH, or the required shell path directly, so the benchmark can log
shell usage.

Task:

Patch `scripts/cleanup.sh` so it removes generated scratch files but preserves
fixtures.

Acceptance:

- Running `./scripts/cleanup.sh` succeeds.
- `fixtures/keep.txt` still exists.
- `build/tmp/generated.txt` is removed.

If the environment provides a dry-run or safety mechanism, use it before running
the cleanup script.

