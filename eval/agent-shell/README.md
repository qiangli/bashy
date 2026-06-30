# Agent Shell Evaluation Pilot

This directory contains the pilot harness for
`docs/plan-agent-shell-evaluation-sprint.md`.

The current harness is intentionally small and is now classified as a
prompt-only shakeout harness. It proves the evaluation loop before a full
fleet/budget run:

- verify both shell arms exist;
- set up deterministic task workspaces;
- launch one agent tool against one task/environment pair;
- run an independent verifier;
- append one JSONL metrics row.

For valid bashy-vs-GNU product claims, use the container harness described in
[`../../docs/agent-shell-eval/container-harness.md`](../../docs/agent-shell-eval/container-harness.md).
The agent CLI must run inside a `bashy podman` container so shell selection is
enforced by the runtime, not by prompt text.

## Required Control Shell For Prompt-Only Shakeout

Set `GNU_BASH53` to a real GNU Bash 5.3 binary before running the `gnu-bash53`
arm:

```sh
GNU_BASH53=/path/to/gnu-bash-5.3 ./eval/agent-shell/run-preflight.sh
```

`bin/bash` in this repository is bashy's pure drop-in and is not the GNU
control. `/bin/bash` on macOS is usually Apple Bash 3.2 and is also not valid.

## Commands

```sh
./eval/agent-shell/run-preflight.sh

./eval/agent-shell/run-task.sh \
  --task wrong-cwd-recovery \
  --env bashy-agentos \
  --tool codex \
  --out results/agent-shell-pilot.jsonl
```

The first conductor drill should use one or two tasks with `codex` and/or
`claude`. Add `agy`, `opencode`, and `aider` only after the logging and verifier
shape is proven.
