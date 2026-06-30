# Retro: 2026-06-30 Container Pilot

## What Worked

- The container harness now compares `bashy v0.4.0` against real GNU Bash 5.3
  under `bashy podman`.
- Per-run summaries were written in `docs/agent-shell-eval/test-*.md`.
- The run captured time, pass/fail, shell invocations, tool calls where
  available, retry count, retry sleep, and API/timeout signals.
- The conductor loop caught and fixed harness defects before treating results as
  product evidence.

## What Did Not Work

- Running the agent CLI inside the container was not practical because the local
  agent binaries are host-native. The pilot used host-agent orchestration with
  container-enforced shell execution.
- The first result JSONL format was too permissive; embedding large nested stream
  JSON broke valid JSONL on Claude rows.
- Claude and agy adapters needed CLI-specific handling before valid runs.
- `agy` can produce very long stalls or timeout/retry cycles. The harness records
  this, but future runs need a clearer per-tool timeout budget.

## Bashy Improvements

- Implemented `-l` / `-lc` handling so common Bash login-command invocations work.
- Implemented `--dry-run` as an alias for `--dryrun`.
- Remaining: make dry-run more discoverable in the front-door help surface.
  Agents still had to probe heavily to find the correct spelling and behavior.

## Conductor Improvements

- Keep separate classes for `valid`, `invalid-harness`, and `tool-adapter-fail`.
  The first raw result file mixed these too loosely.
- Emit curated results after every matrix, not only raw JSONL.
- Keep retry and timeout behavior as first-class metrics; do not hide it as
  noise.
- Sanitize docs before finishing so local paths, hostnames, and usernames do not
  enter repo documentation.

## Next Sprint Recommendations

- Add three harder tasks:
  - dependency/tool availability diagnosis;
  - destructive-script dry-run with manifest parsing;
  - wrong-host or wrong-CWD recovery with misleading logs.
- Add repeated trials per task, at least 3 per tool/shell arm, before claiming a
  statistically meaningful advantage.
- Add a small parser that extracts token/cost summaries from each tool into
  stable fields instead of embedding raw event payloads.
- Consider an optional Linux-agent image path later, but only if auth and install
  costs are understood. The current host-agent/container-shell model is good
  enough for pilot evidence.

