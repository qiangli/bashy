---
id: 1118472f556c
kind: feature
title: 'S227.1 bashy dag build-image: linux FROM-scratch image of the static bashy_scratch artifact that runs a user Bash# script offline'
seq: 305
status: todo
priority: p0
labels:
    - linux
    - oci
    - dag
created: 2026-09-20T21:23:20.094892Z
sprint: 227
---

Today `make build-bashy-scratch` builds the static, purego-free linux/amd64 artifact and `scripts/verify-bashy-scratch.sh` proves it in a throwaway `FROM scratch` image that is then deleted. No dag target and no Makefile target produces a *kept*, runnable image. `examples/quickstart/Containerfile` mode-a is a hello demo with the script baked in and carries a glibc closure because it builds the lean profile, not `bashy_scratch`.

Deliver: a `build-image` target in `dag.md` (Makefile mirror `build-image`) that
- builds `bin/scratch/bashy-linux-<arch>` via the `bashy_scratch` profile for amd64 AND arm64 (extend `build-bashy-scratch` with GOARCH; keep the amd64 artifact path stable — it has a consumer);
- writes `tools/bashy-image/Containerfile` = `FROM scratch` + `/bashy` + `/tmp` + `/work`, `ENTRYPOINT ["/bashy"]`, env `OTEL_TRACES_EXPORTER=none BASHY_TELEMETRY_QUIET=1 HOME=/tmp BASHY_BIN_CACHE=/var/cache/bashy/bin`;
- stages the context the way `tools/bashy-oci/build-smoke.sh` does (public siblings only — never the umbrella parent as raw context);
- tags `localhost/bashy:<version>-linux-<arch>`; `BASHY_IMAGE`, `BASHY_OCI`, `BASHY_OCI_PLATFORM` overrides as in bashy-oci.

User contract (the point of the sprint): `podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:… --bashsharp ./script.bsh`. Document the one-liner in the target's help and README "Containers" section.

Acceptance: image builds on the dev box and on the Linux test host; `--version`, `-c 'printf ok'`, and a mounted `.bsh` using builtin coreutils (ls, sort, grep, awk-free) all pass under `--network=none --read-only --cap-drop=ALL --tmpfs /tmp`; `verify-bashy-scratch.sh` still green; reported compressed + uncompressed size per arch in the evidence record.

**Engine = bashy's own (operator, 2026-09-20 — dogfooding, self-contained).** The build and the smoke run through `bashy podman` (tier 3 `sandbox`), never a host podman/docker: `BASHY_OCI` defaults to `bashy podman` exactly as `test-bash-container` already does (`BASH53_OCI="${BASH53_OCI:-bashy podman}"`); `build-smoke.sh`'s `command -v podman || docker` probe goes away. The dag target body calls `"$BASHY" podman build …` / `"$BASHY" podman run …` so the run stays on the invoking binary. The lean RELEASE bashy already provisions its own engine: `internal/agentos/engines_stub.go` `dispatchEngine` → `provisionEngine` fetches bashy's permissive podman blob (+ gvproxy/vfkit on macOS) through binmgr into `BASHY_BIN_CACHE` — no `make build-host`, no `bashy_engines` tag, no brew. One trap to close here: `resolveEngineBinary` prefers a host `podman` on `$PATH` before the cache, so the self-contained proof (S227.0) runs with `$PATH` scrubbed of podman/docker and `BASHY_PODMAN_SYSTEM` unset; the evidence records `bashy podman path` / `doctor` output naming the managed engine.
