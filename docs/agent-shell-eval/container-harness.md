# Container Harness Requirement

The valid evaluation harness must run tools inside containers launched by
`bashy podman`.

The prompt-only pilot on 2026-06-29 was useful as a harness shakeout, but it is
not strong enough evidence for bashy-vs-GNU claims. The agent could invoke
`/bin/zsh`, absolute shell paths, and CLI defaults. A valid comparison needs the
runtime itself to make the selected shell unavoidable.

## Arms

### bashy-agentos-container

- Evaluated shell: `/usr/local/bin/bashy`.
- Do not use this repo's `bin/bash`; it is only the pure Bash 5.3 drop-in.
- `/bin/sh` and `/bin/bash`, if present, should point to wrappers that log and
  exec `/usr/local/bin/bashy`.
- Agent mode: `DHNT_AGENT=1`, `BASHY_ADVISOR=1`.

### gnu-bash53-container

- Evaluated shell: `/usr/local/bin/bash`.
- Must be real GNU Bash 5.3.
- `/bin/sh` and `/bin/bash`, if present, should point to wrappers that log and
  exec `/usr/local/bin/bash`.
- Agent mode variables for bashy-specific features must be absent/off.

## Host Preflight

The current lean `bin/bashy` build reports:

```text
bashy podman: the container/LLM engines are not in this build
```

Before the next valid run, use one of:

```sh
make build-host
bin/bashy podman info
```

or dispatch the evaluation to a host node whose `bashy` includes the
`bashy_engines` build.

## Validity Rules

- A run without container enforcement is `valid=false` for bashy-vs-GNU product
  claims.
- A run where the agent escapes to a host shell is `valid=false`.
- A run where the bashy arm uses `bin/bash` instead of `bashy` is `valid=false`.
- A run where the GNU arm uses Apple Bash 3.2, bashy `bin/bash`, or any shell
  other than GNU Bash 5.3 is `valid=false`.

## Next Harness Work

- Add `eval/agent-shell/containers/bashy.Containerfile`.
- Add `eval/agent-shell/containers/gnu-bash53.Containerfile`.
- Add a shell wrapper that logs argv/cwd/exit and execs the arm shell.
- Run the agent CLI inside the container, not on the host.
- Mount task workspace at `/workspace` and results at `/results`.
- Disable startup files unless a task explicitly tests startup behavior.

