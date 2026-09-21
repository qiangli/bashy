---
id: a212922db9f0
kind: test
title: 'S227.0 "Bashy is all you need" — the gate: a fresh host with ONLY the bashy release download runs bashy image build + bashy podman run on a .bsh offline (no git/go/cc/podman/docker on the host)'
seq: 312
status: done
priority: p0
labels:
    - airgap
    - self-contained
    - gate
created: 2026-09-20T21:26:22.004144Z
assignee: corbel
sprint: 227
closed: 2026-09-21T07:34:27.747346Z
closed_by: corbel
---

DELIVERED (corbel, 2026-09-21). The sprint's claim as one gate — two lines from the release download:

    bashy self image
    bashy podman run --rm --network=none -v "$PWD:/work" -w /work localhost/bashy:<ver>-linux-<arch> --bashsharp ./script.bsh

`scripts/self-contained-image-smoke.sh vX.Y.Z[-dev]` (dag `smoke-self-contained-image`, `SELF_TAG=`): downloads the tag's archive (the user's one download, verified against `checksums.txt`), builds a PATH that mirrors the system dirs minus git/go/cc/podman/docker (keeping `newuidmap`/`newgidmap` — the stated Linux host fact), empties `BASHY_BIN_CACHE`, runs the two lines on a Bash# script (`func` + typed args + a coreutils pipeline), prints the provision inventory (every file bashy fetched, with sizes + digests). On macOS it inits/starts bashy's own podman machine first (`SELF_MACHINE_OPTS`), records the time, and mounts by physical path (`pwd -P`: the machine shares /Users, /private, /var/folders — /tmp is a host symlink). Never SKIPs.

Candidate `v0.25.0-dev` (tag eea09efa; prerelease with `bashy-scratch-linux-{amd64,arm64}` in checksums.txt). Measured:
- Linux test host (Ubuntu 24.04 amd64), root: archive sha256 verified; line 1 12 s (fetched podman v6.1.2 + the scratch artifact); line 2 `hello, air-gap!` / `z`; inventory 194 868 K; **PASS in 26.6 s**.
- same host, plain user (rootless; `apparmor_restrict_unprivileged_userns=1`): line 1 10 s; `rootless: true`; **PASS in 23.3 s**.
- macOS (arm64; host podman scrubbed off PATH, no machine, image cache moved aside): cold **machine init 28 s + start 12 s** (2 vCPU / 2 GB / 20 GB), line 1 7 s → `localhost/bashy:0.25.0-dev-linux-arm64`; first run failed on the /tmp mount (fixed above), re-run **PASS** (line 1 warm 4 s); inventory 255 892 K (scratch arm64 115 MB, podman client 42 MB, podman-mac-helper 6.6 MB, gvproxy 24 MB, vfkit 64 MB). The intended clean macOS host runs a Homebrew podman machine for other work and was left alone.
- Windows: deferred with reason — no managed Windows podman in this release; a host podman on PATH is what Windows gets today (docs/airgap-image.md says so).

"What the host needs" (docs/airgap-image.md): Linux — `newuidmap`/`newgidmap` + subuid for rootless, overlay/userns kernel support, and Ubuntu 24.04+'s AppArmor userns restriction (bashy prints the way out; some 24.04 kernels do not enforce it — both measured); macOS — nothing but macOS, the machine OS image is podman's own fetch; Windows — WSL2, host podman for now.

Evidence: umbrella docs/sprint-227/evidence.md §S227.0.
