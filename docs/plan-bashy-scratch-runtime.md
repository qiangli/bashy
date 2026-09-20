# Bashy scratch runtime profile (Sprint 218 / Story 559)

## Contract

`make build-bashy-scratch` produces the Cloudbox input at the stable path
`bin/scratch/bashy-linux-amd64`. It is the ordinary lean `cmd/bashy` graph plus
the narrow `bashy_scratch` dependency seam, built with `CGO_ENABLED=0` for
`linux/amd64`. The normal `build`, `build-bashy`, `dist`, install, and GoReleaser
paths are unchanged; this is an explicit deployment profile, not a new release
default.

The profile deliberately adds no OCI framework, supervisor configuration, or
release abstraction. Cloudbox copies this exact file into its separate scratch
image and invokes the already-shipped `bashy supervisord DAG.md TARGET` front
door.

## Verification

`make verify-bashy-scratch` always rebuilds the artifact before checking it. It
fails unless all deterministic checks pass:

- the file is a 64-bit little-endian amd64 ELF executable;
- no ELF `INTERP` program header or `NEEDED` dynamic entry exists;
- an available native `readelf`/`objdump` implementation agrees with the
  host-independent `debug/elf` audit;
- `go list -deps` for the exact scratch profile contains no `purego` package.

On linux/amd64 it executes the artifact directly. On another host it uses a
usable Podman, Docker, or nerdctl runtime to build a literal `FROM scratch`
image. Both execution paths exercise `bashy --version` and
`bashy supervisord --help`. If cross-host execution and a usable runtime are
both unavailable, the command smoke is reported as skipped while the
deterministic structural and dependency checks still gate the artifact. Setting
`BASHY_SCRATCH_RUNTIME` requests a particular runtime and fails closed if that
runtime cannot be used.
