---
id: 3e1fa3fbce08
kind: bug
title: 'uutils scoreboard cannot complete on a 2 vCPU / 8 GB host: /work tmpfs (size=8g, holds the cargo target dir) counts against the container --memory cgroup -> OOM'
seq: 362
status: todo
priority: p2
labels:
    - uutils
created: 2026-09-30T13:50:55.68131Z
---

Found 2026-09-30 in Sprint #110 (34046901) on s110-cert-amd64 (s-2vcpu-8gb-160gb-intel, +8G swap) against the frozen candidate (SUT digest 0844a95e). make test-uutils: container exit 101 at UUTILS_MEMORY=3g and again at 6g; dmesg: memcg OOM killed the uutils tests binary (anon-rss ~2.3 GB) after ~240 of 5366 listed tests, because scripts/uutils-oci-lib.sh mounts --tmpfs /work size=8g (cargo target) inside the same memory cgroup. No uutils scoreboard has ever been recorded (S119/S269 did not run it), so there is no baseline. Options: run on a >=16 GB host, or put the cargo target on a disk-backed (non-tmpfs) volume. Partial transcript kept at dragon ~/.bashy/sprint/110/evidence/gates/uutils-run-oom.txt (sha256 2df150c8).
