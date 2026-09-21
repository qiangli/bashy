---
id: a212922db9f0
kind: test
title: 'S227.0 "Bashy is all you need" — the gate: a fresh host with ONLY the bashy release download runs bashy image build + bashy podman run on a .bsh offline (no git/go/cc/podman/docker on the host)'
seq: 312
status: doing
priority: p0
labels:
    - airgap
    - self-contained
    - gate
created: 2026-09-20T21:26:22.004144Z
sprint: 227
---

The sprint's claim as one gate. Download bashy; two lines; your `.bsh` runs offline in a container — no git, go, cc, podman or docker on the host:

    bashy image build
    bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh

Everything else is bashy provisioning itself into `BASHY_BIN_CACHE`: the podman engine (S227.8, from the pinned tag `engines-v1`: podman + gvproxy on linux, + vfkit on macOS) and the scratch artifact (S227.1, from the same release as the running bashy). On macOS and Windows the linux image runs inside bashy's own podman machine (vfkit / WSL2 — WSL2 exists on every Windows edition); there is no per-OS image or bundle (S227.5, S227.6 deferred).

Deliver `scripts/self-contained-image-smoke.sh` (dag `smoke-self-contained-image`): starts from a release archive on a host with `$PATH` = the bashy install dir + minimal system dirs (no git/go/cc/podman/docker), `BASHY_PODMAN_SYSTEM` unset, `BASHY_BIN_CACHE` = an empty dir; runs the two lines; prints the cache inventory (what bashy fetched, with digests) — that inventory IS docs/airgap-image.md §"What the host needs".

Prerequisites, stated not hidden, in that section: Linux rootless podman needs `newuidmap`/`subuid` (record per distro, or state rootful); the podman machine OS image is fetched by podman itself on `machine init` (macOS/Windows — bashy's engine fetching, under upstream terms, listed in the inventory, never redistributed by bashy); Windows: a host podman on `$PATH` in 227 (no managed Windows blob yet — recorded).

Needs a `-dev` tag carrying S227.8 + S227.1 first: the gate runs from published bytes.

Acceptance: passes on the Linux test host and a macOS host from published bytes with the scrubbed `$PATH`; Windows on the QA host with a host podman, or a recorded reason; inventory per OS pasted into the doc; nothing the host needed is left unnamed.
