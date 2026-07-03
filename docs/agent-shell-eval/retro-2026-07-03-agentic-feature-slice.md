# Agentic Feature Slice Retro - 2026-07-03

## Run Summary

The batch ran two bashy-agentic tasks across three tools and two shell arms:

- `agentic-dryrun-cleanup`
- `agentic-cwd-advisor`
- tools: `codex`, `claude`, `agy`
- shell arms: `bashy-current`, `gnu-bash53`

Result: `12/12` selected valid runs passed. One invalid pre-fix harness run is
kept in raw JSONL and excluded from the aggregate.

## What Worked

- The container-enforced harness still works with current bashy after rebuilding
  both the host engine binary and the Linux arm64 image binary.
- Passing verifier logs into the workspace made it possible to test actual agent
  behavior, not only final filesystem state.
- All three agents discovered bashy's dry-run surface when the task/verifier made
  it required.
- Workspaces were kept under `/private/tmp/bashy-eval-runs`, avoiding new
  generated benchmark trees in the repo.

## What Failed Or Was Weak

- The first dry-run attempt exposed a macOS path canonicalization bug:
  `/tmp/...` became `/private/tmp/...` in process cwd, while the Podman remote
  later tried to stat the non-canonical path during verification.
- The cwd-advisor task was too easy. Agents solved it on both shells without
  enough friction to show whether bashy's advisor materially helped.
- The dry-run task gives bashy extra safety work, so it is good for verifying
  safety affordance discovery but poor for wall-time comparison.
- The `valid` heuristic for Codex remains conservative and should be reviewed
  if host-side tool-call detection changes.

## Product Findings

- Bashy's dry-run mode is discoverable enough for `codex`, `claude`, and `agy`
  when the prompt says to use it and the verifier enforces it.
- Agentic features need benchmark tasks where the feature removes work, not only
  tasks where the feature adds a safety requirement.
- The next differentiating tasks should use structured bashy outputs:
  `bashy check --agent --json`, `bashy run --check --json`, and
  `bashy commands COMMAND --features`.

## Follow-Ups

- Add generated task support so agentic-feature tasks can be selected from a
  declarative manifest instead of hand-written directories.
- Tighten the cwd-advisor benchmark by starting agents in a misleading nested
  directory with similarly named local files, so a wrong-cwd hint has measurable
  value.
- Add an explicit dry-run metric to the JSONL output rather than requiring each
  task verifier to grep the command log.
- Decide whether per-run markdown summaries should stay in `docs/` or move to a
  local-only results area with only aggregate reports committed.
