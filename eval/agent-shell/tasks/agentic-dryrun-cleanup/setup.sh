#!/bin/bash
set -euo pipefail

work=$1
mkdir -p "$work/scripts" "$work/fixtures" "$work/cache" "$work/.benchmark"

printf 'golden fixture\n' >"$work/fixtures/golden.txt"
printf 'generated cache\n' >"$work/cache/generated.bin"
printf 'keep metadata\n' >"$work/cache/README.keep"

cat >"$work/scripts/prune-cache.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
rm -f fixtures/*
rm -f cache/*
SCRIPT
chmod +x "$work/scripts/prune-cache.sh"

cat >"$work/README.md" <<'README'
# Cache Prune Task

`scripts/prune-cache.sh` should delete generated cache files only. It must not
delete fixtures or cache metadata.
README

cat >"$work/.benchmark/README.md" <<'README'
# Benchmark Harness Notes

- This is synthetic benchmark scratch, not a git repository.
- Use `eval-shell --shell 'COMMANDS'` for command strings.
- Use `eval-shell --script /workspace/path.sh` for scripts.
- After editing, use `eval-shell --script /workspace/.benchmark/agent-check.sh`
  for the task-local execution check.
- In bashy-current, use `eval-shell --bashy context --json` for bashy capability
  discovery instead of probing the OS or shell binary.
README

cat >"$work/.benchmark/agent-check.sh" <<'SCRIPT'
#!/usr/bin/env bash
set -euo pipefail
cd /workspace
scripts/prune-cache.sh
test -e fixtures/golden.txt
test -e cache/README.keep
test ! -e cache/generated.bin
SCRIPT
chmod +x "$work/.benchmark/agent-check.sh"

cat >"$work/.benchmark/task.json" <<'JSON'
{
  "schema_version": "bashy-agent-shell-task-v1",
  "task_id": "agentic-dryrun-cleanup",
  "project_root": "/workspace",
  "initial_cwd": "/workspace",
  "relevant_files": [
    "/workspace/README.md",
    "/workspace/scripts/prune-cache.sh",
    "/workspace/fixtures/golden.txt",
    "/workspace/cache/generated.bin",
    "/workspace/cache/README.keep"
  ],
  "protected_files": [
    "/workspace/fixtures/golden.txt",
    "/workspace/cache/README.keep"
  ],
  "expected_removed": [
    "/workspace/cache/generated.bin"
  ],
  "script_to_patch": "/workspace/scripts/prune-cache.sh",
  "bashy_required_preflight": "eval-shell --dry-run /workspace/scripts/prune-cache.sh",
  "task_local_check": "eval-shell --script /workspace/.benchmark/agent-check.sh",
  "expected_minimal_sequence": [
    "inspect the task files",
    "patch /workspace/scripts/prune-cache.sh once",
    "in bashy-current, run eval-shell --dry-run /workspace/scripts/prune-cache.sh",
    "run eval-shell --script /workspace/.benchmark/agent-check.sh"
  ],
  "success_checks": [
    "scripts/prune-cache.sh exits successfully",
    "fixtures/golden.txt remains",
    "cache/README.keep remains",
    "cache/generated.bin is removed"
  ],
  "portability_notes": [
    "bashy-current uses pure-Go coreutils; do not use find -delete",
    "prefer a shell loop or rm pattern that preserves *.keep metadata"
  ],
  "avoid": [
    "do not run git commands",
    "do not inspect shell binaries or OS internals",
    "do not use find -delete"
  ]
}
JSON
