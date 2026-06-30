# Sprint: Bashy Tool Improvements From Container Pilot

Date: 2026-06-30

Goal: implement the highest-signal bashy improvements identified by the
container pilot retro, especially issues that caused agents to spend extra time
discovering bashy capabilities.

## Scope

- Improve dry-run discoverability.
- Keep `bin/bash` drop-in behavior clean; bashy-only help must be wired only
  through `cmd/bashy`.
- Improve agent-facing command discovery.
- Reduce `bashy podman` resource-probe noise when agent harnesses narrow `PATH`.

## Conductor Note

This sprint was implemented solo because the selected improvements were small
and tightly coupled. For larger bashy improvement sprints, the conductor should
use `bashy weave` to split independent work into isolated agent workspaces,
assign fleet members (`codex`, `claude`, `agy`, and, after budget approval,
`opencode`/`aider`), review their patches, and merge only accepted changes back
into the main bashy repo.

## Implemented

- Added `cli.AgentOSUsage` so `bin/bashy --help` can append bashy-only help while
  `bin/bash --help` remains GNU-compatible.
- Added `bashy help dryrun`.
- Added `bashy commands --agentic`.
- Kept `--dry-run` as a first-class documented alias for `--dryrun`.
- Added unit coverage for the dry-run alias and agentic discovery text.
- In sibling `coreutils`, made Darwin podman resource probing fall back to
  `/usr/sbin/sysctl` and `/usr/bin/vm_stat` when `PATH` is minimal.

## Verification

```sh
env GOCACHE=/private/tmp/bashy-gocache go test ./internal/agentos ./internal/cli
env GOCACHE=/private/tmp/coreutils-go-build go test ./external/podman/engine
make build-host VERSION=v0.4.0
bin/bashy --help
bin/bashy help dryrun
bin/bashy commands --agentic
env PATH=/nonexistent bin/bashy podman image exists bashy-agent-shell:bashy-v0.4.0
```

## Notes

- The podman probe fix belongs to the `coreutils` sibling because that package
  owns the embedded podman engine.
- The help improvements belong in `bashy` because they are shell/front-door UX.
