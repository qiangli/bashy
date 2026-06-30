# Container Preflight 2026-06-29

## Summary

Built and verified the shell-arm container substrate for the agent shell
evaluation.

This was not an agent benchmark run. It verified that the next benchmark can
force the shell choice at container level.

## Build Under Test

- Version stamp: `v0.4.0`
- Local host binary: `bin/bashy`
- Host build: `make build-host VERSION=v0.4.0`
- Container worker binary: Linux/arm64 `bashy` built from `./cmd/bashy`

## Images

- `bashy-agent-shell:bashy-v0.4.0`
- `bashy-agent-shell:gnu-bash53`

## Commands

```sh
make build VERSION=v0.4.0
make build-host VERSION=v0.4.0
env CGO_ENABLED=0 GOOS=linux GOARCH=arm64 \
  go build -trimpath \
  -ldflags "-s -w -X 'github.com/qiangli/bashy/internal/cli.bashVersion=5.3.0(1)-bashy-v0.4.0'" \
  -o ~/tests/bashy-eval/bin/bashy-linux-arm64 ./cmd/bashy
./eval/agent-shell/container-preflight.sh ~/tests/bashy-eval
```

## Result

```text
container_preflight=pass
bashy_image=bashy-agent-shell:bashy-v0.4.0
gnu_image=bashy-agent-shell:gnu-bash53
```

Version checks:

```text
GNU bash, version 5.3.0(1)-bashy-v0.4.0
GNU bash, version 5.3.0(1)-release (aarch64-unknown-linux-gnu)
```

Invocation checks:

```text
bashy-lc-ok
gnu-lc-ok
```

## Timings

- `make build VERSION=v0.4.0`: about 2 seconds.
- `make build-host VERSION=v0.4.0`: about 16 seconds.
- Linux/arm64 `bashy` worker build: about 6 seconds.
- Container preflight: about 21 seconds on rerun after cached Debian dependency
  layers. First attempt downloaded Debian packages and failed at the Bash source
  symlink copy before the script was fixed.

## Harness Fixes

- `external/bash-5.3` is a symlink. The first GNU image build copied the symlink
  into the podman build context and failed. Fixed
  `eval/agent-shell/container-preflight.sh` to dereference the source tree with
  `cp -RL "$repo/external/bash-5.3/."`.

## Next Test Estimate

One container-enforced agent test:

- tools: `codex`
- envs: `bashy-agent-shell:bashy-v0.4.0`, `bashy-agent-shell:gnu-bash53`
- tasks: `wrong-cwd-recovery`
- runs: 2
- estimated wall time: 5-15 minutes now that images exist
- token range: 40k-180k input, 1k-8k output per run
- paid budget exposure: none beyond subscription
- rate-limit/API risk: possible; retries/backoff must be recorded
- approval required: no for `codex`; yes before `opencode`/`aider`

## Status

Ready to implement the container-enforced agent runner for a single `codex`
paired test.

