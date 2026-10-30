---
id: 64d907c279e5
kind: task
title: S531.D3w [SUPERSEDED by D3x] bashy oci auto-provision for genie exec on Windows (podman design rejected by operator 20:46Z)
seq: 2839
status: wontfix
priority: p0
created: 2026-10-29T20:33:07.611486Z
assignee: claude-opus5.5
sprint: 531
sprint_id: f7bc2570-ca8e-59b0-8163-c3c51ba23610
sprint_title: 'Genie core harness: drive vendor agent CLIs as tools'
closed: 2026-10-29T22:29:24.008238Z
---

Found in D3 Windows runs 7+8 10-29, candidate bashy f413bdcb on elephant native Windows. genie policy refused plain 'sh test_slug.sh' and 'git commit' (incomplete), and the remedy wrapped them in @contain(net: deny). bashy contain then failed: 'bashy podman is not running (create its VM once: bashy podman machine init && bashy podman machine start)', exit 125. On WSL/Linux the same commands run (native network namespace). Operator principle: same on every OS, zero user setup; a manual podman init is a bug. DECIDED with s525-manager (mb:10604): ONE mechanism in bashy oci. On first use, bashy oci (the image provider behind bashy contain AND yoke pkg/shape/contain/oci.go's BashyOCI for @sandbox) provisions and starts its isolated 'bashy' machine with no prompt (Windows: the WSL2 backend; keep the CONTAINERS_HELPER_BINARY_DIR/winhelper provisioning; macOS: as today), idempotently and safely under concurrent first calls (a lock), with a clear progress line on stderr and a bounded timeout. NEVER run podman on Dragon: unit tests use a fake machine/engine seam; live proof on elephant native Windows only (claimed by s531-manager; use only C:/Users/liqia/s531-d3; an outpost-ssh run of a cross-compiled test or candidate binary staged there is OK). Red/green: unit test that contain/oci auto-inits and starts a missing machine through the seam and does not re-init a running one; live: 'bashy contain --net deny -- cmd /c echo ok' on elephant with no machine -> ok. Focused go test only. s525 A5w-oci's Windows proof depends on this.
