# Container Harness Requirement

The valid evaluation harness must run tools inside containers launched by
`bashy podman`.

The prompt-only pilot on 2026-06-29 was useful as a harness shakeout, but it is
not strong enough evidence for bashy-vs-GNU claims. The agent could invoke
`/bin/zsh`, absolute shell paths, and CLI defaults. A valid comparison needs the
runtime itself to make the selected shell unavoidable.

## Arms

### bashy-current

- Evaluated shell: `/usr/local/bin/bashy`.
- Image tag: `bashy-agent-shell:bashy-current`.
- Do not use this repo's `bin/bash`; it is only the pure Bash 5.3 drop-in.
- `/bin/sh` and `/bin/bash`, if present, should point to wrappers that log and
  exec `/usr/local/bin/bashy`.
- Agent mode: `DHNT_AGENT=1`, `BASHY_ADVISOR=1`.
- Record the exact `bashy --version` output for every preflight because this
  arm tracks the current development build, not a fixed release tag.

### gnu-bash53-container

- Evaluated shell: `/usr/local/bin/bash`.
- Must be real GNU Bash 5.3.
- `/bin/sh` and `/bin/bash`, if present, should point to wrappers that log and
  exec `/usr/local/bin/bash`.
- Agent mode variables for bashy-specific features must be absent/off.

## Host Preflight

The current lean `bin/bashy` build reports:

The default release-style `bin/bashy` build is intentionally lean. If
`bashy podman` reports that the container/LLM engines are not in this build,
use a host build with engines enabled before running the container harness.

Before the next valid run, use one of:

```sh
make build-bashy BASHY_ENGINES=1 VERSION=eval-$(git rev-parse --short=12 HEAD)
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

- Keep `bashy-current` as the live development arm and preserve fixed release
  tags only for historical replay.
- Add more tasks that expose bashy differentiators, especially shell-check,
  on-demand fallback, advisor hints, and self-build workflows.
- Add per-run retro notes that convert bashy shortcomings into concrete product
  work.
