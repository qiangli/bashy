# Retro: 2026-07-01 Current Baseline Eval

## What Worked

- The refreshed `bashy-current` arm made the run honest: new results no longer
  reuse the stale `bashy-v0.4.0` image label.
- Rebuilt host `bin/bashy` successfully ran `bashy podman` with embedded
  podman/vfkit/gvproxy, and the container preflight passed.
- All 12 runs completed without harness intervention, retries, invalid shell
  escapes, or verifier failures.
- `agy` on bashy-current used `--dryrun` quickly on `dryrun-safe-edit`, while
  the GNU Bash arm spent much longer probing and rerunning commands.

## What Should Improve

- The `dryrun-safe-edit` verifier checks only final state. It does not enforce
  that an agent used dry-run before the destructive run, even though the prompt
  asks for it when available. That makes the current pass/fail too coarse for a
  safety-oriented task.
- The command-count metric currently treats the verifier invocation as a bash
  command. That is consistent across arms, but future reporting should split
  agent commands from verifier commands.
- Claude telemetry still causes occasional API/rate-limit signal counts even
  when the event is non-fatal and no retry occurs. The parser should classify
  `status:"allowed"` separately from actual failures.
- agy token usage remains `not_parsed`; this limits cross-tool token
  comparisons.
- The GNU Bash control image was sensitive to copied build residue from
  `external/bash-5.3`. The container now deletes object/archive artifacts, but
  the broader rule should be documented for any future source-built controls.

## Bashy Product Signals

- `bashy --dryrun` is discoverable enough for agy in the current build and gave
  the clearest observed advantage in this batch.
- Codex still probes bashy with `--help`, `bash -lc`, `sh -c`, and `--dryrun`
  before converging. That suggests help output and task-facing hints matter for
  agent efficiency.
- The current run did not exercise newer shell-check, command inventory, or
  on-demand fallback features. The next task set should target those directly.

## Conductor Notes

- Running the full 12-run matrix sequentially was acceptable after image
  preflight; total agent wall time was 526s across both arms.
- The preflight-first approach caught two real harness issues before spending
  agent tokens on invalid runs.
- The next batch should define safety behavior in the verifier, not just in the
  prompt, so pass/fail reflects the behavior we actually care about.

## Follow-Up Work

- Add a stricter safety task where the verifier fails if a destructive command
  runs before a dry-run/audit step.
- Split `bash_command_invocations` into `agent_bash_invocations` and
  `verifier_bash_invocations`.
- Parse agy token usage or record transcript-token estimates separately.
- Refine Claude rate-limit parsing to distinguish allowed telemetry from
  blocking/API failures.
- Add bashy-differentiator tasks for `bashy check`, `bashy commands`, on-demand
  fallback, advisor hints, and self-build.
