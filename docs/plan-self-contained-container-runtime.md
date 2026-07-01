# Plan: self-contained container runtime for bashy

Status: planned after the `bashy check` sprint exposed a false conformance
score. The local lean `bin/bashy` could not run `bashy podman`, so POSIX
oracle harnesses either failed or reported a bogus `0/0` scoreboard.

## Goal

`bashy` should be self-contained for agent and conformance work without forcing
every release binary to embed a large cgo podman engine.

The desired user experience:

```sh
bashy podman run --rm bash:5.3 bash --version
bashy docker run --rm bash:5.3 bash --version
bashy check --allow-container script.sh
```

If a managed embedded engine is compiled in, use it. If not, `bashy podman`
should resolve or install a managed external container runtime and run the same
command. If neither path can work on the host, fail loudly with a diagnostic
that tells the user which prerequisite is missing.

## Design Decision

Use an on-demand managed-external runtime instead of embedding podman in every
default release.

Reasoning:

- The current lean `bashy` is pure Go and cross-compiles to Linux, macOS, and
  Windows with `CGO_ENABLED=0`. That is valuable for outposts and bootstrap.
- The full embedded podman engine is unix-host only and heavy. Making it the
  default would break the current release contract and still not solve Windows.
- `coreutils/pkg/binmgr` already gives bashy a verified download/cache path for
  tools such as `gh`, `act`, and `go`. Container runtimes should use the same
  trust and cache model.

## Runtime Resolution Order

`bashy podman ...` should resolve in this order:

1. `BASHY_PODMAN_SYSTEM=1`: explicit host passthrough to `podman` on `PATH`.
2. Embedded engine: current `-tags bashy_engines` path on supported unix hosts.
3. Managed external podman client/runtime from the bashy cache.
4. Host runtime fallback, if present: `podman` then `docker`.
5. Clear failure with remediation:
   - `bashy podman install`
   - `bashy doctor`
   - host/OS-specific note if containers are unsupported.

The `docker` shell shim should continue to route to `bashy podman`, so scripts
that ask for Docker-compatible commands can still use the managed path.

## Subcommands

Add a small management surface:

```sh
bashy podman install       # ensure managed runtime bits are present
bashy podman path          # print resolved runtime binary/socket info
bashy podman doctor        # diagnose engine availability and VM/socket state
bashy podman system        # run host podman explicitly, equivalent to env gate
```

`install`, `path`, and `doctor` are bashy-owned helpers. All other args remain a
transparent podman-compatible passthrough.

## Managed Runtime Options

Phase 1 should use the lowest-risk available runtime per platform:

- macOS: download/cache the upstream Podman client and use `podman machine` when
  available. If the current embedded/vfkit path is present, prefer it.
- Linux: use managed Podman when feasible, otherwise host podman/docker. Rootless
  support should be diagnosed explicitly.
- Windows: reuse upstream Podman's machine stack instead of inventing a
  separate backend. The vendored provider path already supports WSL and Hyper-V,
  but the bashy build must be the remote-machine/client form, not the unix
  in-process libpod form:

  ```sh
  GOOS=windows GOARCH=amd64 CGO_ENABLED=0 \
    go build -tags 'bashy_engines remote containers_image_openpgp' ./cmd/bashy
  ```

  `remote` keeps unix-only in-process libpod out of the Windows binary.
  `containers_image_openpgp` avoids the cgo-only gpgme signing dependency.
  A Windows build with only `bashy_engines` intentionally keeps the current
  clear stub instead of failing in upstream unix-only packages.

  The first Windows enablement patch wires `bashy podman` for that explicit tag
  set, adds real Windows memory/disk probes for machine sizing, and downloads
  the upstream Podman remote-client archive on demand via `binmgr`.

Longer term, the managed package may include full tree assets, helper binaries,
and VM support using binmgr's tree extraction mode.

## Windows Podman Backend Notes

The upstream code already has the pieces bashy needs:

- `pkg/machine/provider/platform_windows.go`: provider selection, defaulting to
  WSL with Hyper-V as another supported provider.
- `pkg/machine/wsl` and `pkg/machine/hyperv`: machine lifecycle backends.
- `pkg/machine/machine_windows.go`: named-pipe API forwarding and
  `win-sshproxy.exe` launch logic.
- `pkg/machine/gvproxy_windows.go`: Windows network helper integration.

The bashy/coreutils integration should therefore adapt those upstream paths
through `github.com/qiangli/coreutils/pkg/oci/machine` instead of copying a
second backend into bashy.

Implemented first slice:

- Windows engine build dispatch:
  `-tags 'bashy_engines remote containers_image_openpgp'`.
- Managed Podman remote-client download/cache through `binmgr`, verified from
  the release `shasums`.
- Whole-tree extraction on Windows so `podman.exe`, `gvproxy.exe`, and
  `win-sshproxy.exe` are cached together.
- Child PATH prepending so Podman can find the helper binaries beside the
  managed client.
- Git-Bash/Cygwin-style shell path presentation on Windows:
  `C:\Users\name` is exposed as `/c/Users/name`, while bashy's syscall layer
  converts `/c/...` back to Windows paths.

Remaining runtime work:

- Add `bashy podman doctor` checks for WSL2 availability, Hyper-V availability,
  Windows virtualization support, and whether the current user can use the
  chosen provider.
- Prove `podman machine init/start` on both Windows hosts. Non-mutating
  `podman machine list` works on `lj2ivy`; a machine has not been initialized in
  this patch.
- Add a fallback that builds helper binaries from the vendored Podman source
  when a release archive is unavailable and Go is present.
- Teach `bashy podman path` to report the provider, helper locations, machine
  socket/pipe, and whether it is using embedded, managed, or host bits.
- Validate Windows path/volume behavior. The first supported contract can mount
  the current working directory into `/workspace`; full absolute path argument
  translation can follow later.

## Container Fallback For Missing Commands

`bashy check --allow-container` currently classifies missing GNU coreutils as
available through a container fallback; it does not execute them.

This should become the planning layer for on-demand provisioning. Before an
agent runs a script, `bashy check` can identify the exact command closure and
tell the runtime layer which support must be present:

- bashy-native commands and builtins: no provisioning.
- missing GNU coreutils: require the managed GNU userland image.
- git features not covered by embedded git, such as submodule checkout: require
  containerized or managed external git.
- dynamic command names: require human/agent review or a permissive execution
  policy.
- system PATH commands: allowed, denied, or replaced by managed fallback based
  on policy.

After the runtime resolver exists, add:

```sh
bashy coreutils install
bashy coreutils run -- COMMAND ARG...
```

Behavior:

- Pull or build a small GNU userland image, probably Alpine or Debian slim.
- Mount the current working directory at the same path when possible.
- Preserve stdin/stdout/stderr and exit status.
- Make dry-run/check output say exactly when a command will run in-process,
  through managed container fallback, through host PATH, or fail.

This also gives the git-submodule escape hatch: when bashy's embedded git lacks
a feature, run a containerized system git with the current repository mounted.

The agent-facing flow should be:

```sh
bashy check --json --allow-container script.sh
bashy check --install-plan script.sh      # proposed follow-up: print required managed assets
bashy coreutils install                   # or bashy support install, see naming below
bashy script.sh
```

`--install-plan` should be a no-mutation mode. It lets an agent ask for approval
or estimate runtime/network cost before downloading images or external tools.

## Conformance Harness Requirements

Harnesses must never emit success-looking bogus scores when the oracle runtime
is unavailable.

Rules:

- If the oracle container command fails, exit `2`.
- If no verdict/probe markers are produced, exit `2`.
- Do not suppress the runtime stderr that explains the failure.
- Report compatibility and conformance scores only from non-empty verdict sets.

The first hardening patch fixes this for:

- `scripts/posix-parity.sh`
- `scripts/yash-scoreboard.sh`

## Implementation Phases

1. Harness honesty:
   - Fix `yash-scoreboard.sh` to call `$OCI ps/rm`, not `$OCI podman ps/rm`.
   - Validate non-empty oracle output in `posix-parity.sh` and `yash-scoreboard.sh`.
   - Gate with `go test ./...`, `make test-bash-parallel`, and CI.

2. Runtime resolver:
   - Move podman fallback logic into `coreutils/external/podman`.
   - Add managed `Ensure` support using `pkg/binmgr`.
   - Keep embedded engine as preferred when `bashy_engines` is compiled in.
   - Add unit tests for resolution order without downloading.

3. Runtime management UX:
   - Add `bashy podman install/path/doctor`.
   - Update `bashy doctor` to report embedded, managed, host, and unavailable
     states separately.

4. Harness migration:
   - Standardize conformance scripts on an `OCI` contract:
     `OCI run ...`, `OCI build ...`, `OCI image exists ...`, `OCI ps ...`.
   - Add a small shared helper only if duplication starts causing drift.

5. Containerized missing-tool fallback:
   - Add `bashy coreutils install/run`.
   - Connect `bashy check --allow-container` diagnostics to the actual fallback
     command name and image.

## Non-Goals

- Do not make the pure `cmd/bash` drop-in depend on containers, coreutils, or
  AgentOS.
- Do not require every default release artifact to embed podman.
- Do not hide a missing oracle as `0/0`, `0 match`, or any other score-like
  output.
