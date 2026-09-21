---
id: a212922db9f0
kind: test
title: 'S227.0 "Bashy is all you need" — self-contained proof: a fresh host with ONLY the bashy release download builds the air-gapped image (no git, go, cc, podman or docker on the host)'
seq: 312
status: todo
priority: p0
labels:
    - airgap
    - self-contained
    - gate
created: 2026-09-20T21:26:22.004144Z
sprint: 227
---

The premise of the sprint, stated as one gate. It is the same story as bashy's self-build (README "From source": `bashy git clone` → `bashy scripts/bootstrap-siblings.sh` → `bashy dag build` through `bashy go`), extended by one rung: `bashy dag build-image` through `bashy podman`. Everything the host would normally provide is a bashy built-in that provisions itself on first use into `BASHY_BIN_CACHE` — git (MinGit on Windows; on unix the platform git is used today, see below), the Go toolchain (`bashy go`), the container engine (`provisionEngine` in `internal/agentos/engines_stub.go`: bashy's permissive podman blob + gvproxy/vfkit on macOS, via binmgr from bashy's own release).

The user story, verbatim: *download bashy; run five lines; get an image that runs your .bsh offline.*

```sh
bashy git clone https://github.com/qiangli/bashy && cd bashy
bashy scripts/bootstrap-siblings.sh
bashy dag build-image EXTERNALS="python"     # S227.1 + S227.2, engine = bashy podman
bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:… --bashsharp ./script.bsh
```

Deliver `scripts/self-contained-image-smoke.sh` (dag `smoke-self-contained-image`): starts from a release archive (the `-dev` tag's published bytes, per the release runbook), on a host or VM prepared with NOTHING else — `$PATH` = the bashy install dir + the minimal system dirs, explicitly without git/go/cc/podman/docker; `BASHY_PODMAN_SYSTEM` unset; `BASHY_BIN_CACHE` pointed at an empty dir so every provision is observed and listed. Prints the cache inventory at the end (what bashy fetched, with digests) — that inventory is also the answer to "what does the host really need".

Blocking dependency found at review (2026-09-20): the third line fails today on EVERY platform — the latest release (`v0.24.9`) carries no `podman-<goos>-<goarch>.gz`, so `provisionEngine` has nothing to fetch (only `v0.9.0` ever got blobs, darwin/arm64 only; `engine-blobs.yml` never auto-fires and has no linux runner). S227.8 fixes that and precedes this story. Also: this gate runs from PUBLISHED bytes, so a `-dev` tag carrying S227.1 + S227.2 + S227.8 must be cut first (two-tag release mechanics: `-dev` then promote) — the release step is on the execution order, not implied.

Known gaps this story must resolve or record, not paper over:
- unix `bashy git` deliberately uses the platform git (README table: macOS = Xcode CLT, Linux = distro git). Either the gate accepts "git present" as the one unix prerequisite and says so in the doc, or `bashy git` gains the managed-git rung on unix too — decision to the operator, recorded on this story before claiming.
- `resolveEngineBinary` prefers a host podman on `$PATH` (tier 3) over the cache: the gate scrubs `$PATH`, and the doc says the managed engine is what a clean host gets.
- Linux rootless podman needs `newuidmap`/`subuid` on the host — record whether the managed blob's rootless mode works on the test host's distro or whether rootful is the stated requirement.
- macOS: the managed podman needs a `podman machine` (vfkit VM) — first `bashy podman build` initializes it; time and disk recorded.

Acceptance: the smoke passes on Linux (test host) and macOS from published bytes with the scrubbed PATH; the printed provision inventory is pasted into docs/airgap-image.md §"What the host needs"; any prerequisite that remains (e.g. unix git) is named there with the reason. Windows: same script shape via S227.5's host, or a recorded reason it is deferred.
