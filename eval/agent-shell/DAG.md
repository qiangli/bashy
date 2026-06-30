---
name: agent-shell-eval
description: Remote/container pilot targets for the agent shell evaluation campaign
default: list
---

# Agent Shell Evaluation DAG

This DAG is the control-plane entrypoint for running evaluation pilots locally
or on a caller-selected remote host.

Large artifacts must move over the local P2P network or be fetched by the
remote body. `bashy dag --mesh` only sends the command body over SSH; it should
not be treated as a bulk transport.

## list
Show available evaluation targets.

```bash
echo "targets: preflight-remote one-run-estimate"
```

## one-run-estimate
Print the current one-run pilot estimate. This target is local and safe.

```bash
cat <<'ESTIMATE'
Estimate:
- tools: codex
- envs: bashy-agentos-container, gnu-bash53-container
- tasks: wrong-cwd-recovery
- runs: 2
- wall time: 2-8 minutes total on a prepared high-capacity remote host after images exist
- tokens: 40k-180k input, 1k-8k output per run
- paid budget: none beyond subscription
- approval required: no for codex; required before opencode/aider
ESTIMATE
```

## preflight-remote
Check whether the current execution host can run the container harness. To run
this remotely, select the host outside this DAG with `bashy dag --mesh` and a
local environment/SSH config; do not hard-code machine names in the DAG.

```bash
set -e
uname -a
echo "cores=$(sysctl -n hw.ncpu 2>/dev/null || true)"
echo "mem_bytes=$(sysctl -n hw.memsize 2>/dev/null || true)"
echo "bashy=$(command -v bashy || true)"
if command -v bashy >/dev/null 2>&1; then
  bashy --version | sed -n '1p'
  bashy podman info >/tmp/agent-shell-eval-podman-info.txt 2>&1 \
    && echo "bashy_podman=ok" \
    || { echo "bashy_podman=fail"; sed -n '1,80p' /tmp/agent-shell-eval-podman-info.txt; }
else
  echo "bashy=missing"
fi
echo "podman=$(command -v podman || true)"
if command -v podman >/dev/null 2>&1; then
  podman info >/tmp/agent-shell-eval-host-podman-info.txt 2>&1 \
    && echo "host_podman=ok" \
    || { echo "host_podman=fail"; sed -n '1,80p' /tmp/agent-shell-eval-host-podman-info.txt; }
fi
```
