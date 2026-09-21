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

IN DELIVERY (corbel, 2026-09-21). The sprint's claim as one gate — two lines from the release download:

    bashy self image
    bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh

`scripts/self-contained-image-smoke.sh vX.Y.Z[-dev]` (dag `smoke-self-contained-image`, `SELF_TAG=`): downloads the tag's archive (the user's one download, verified against `checksums.txt`), builds a PATH that mirrors the system dirs minus git/go/cc/podman/docker (keeping `newuidmap`/`newgidmap` — the stated Linux host fact), empties `BASHY_BIN_CACHE`, runs the two lines on a Bash# script (`func` + typed args + a coreutils pipeline), and prints the provision inventory (every file bashy fetched, with sizes + digests) — the doc's "What the host needs". On macOS it inits/starts bashy's own podman machine first (`SELF_MACHINE_OPTS` sizes it) and records the time. Never SKIPs.

Candidate: `v0.25.0-dev` (tag eea09efa). Runs: Linux test host as root and as a plain user; macOS on the dev box (the intended clean macOS host turned out to run a Homebrew podman machine that must not be disturbed — recorded); Windows deferred with reason (no managed Windows podman in this release; host podman path is what Windows gets today — docs/airgap-image.md).

Results: see the evidence record (umbrella docs/sprint-227/evidence.md §S227.0) — filled as the runs land.
