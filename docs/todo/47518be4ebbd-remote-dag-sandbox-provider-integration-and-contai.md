---
id: 47518be4ebbd
kind: feature
title: Remote DAG sandbox provider integration and container dogfood
seq: 377
status: todo
priority: p2
labels:
    - remote
    - sandbox
created: 2026-09-30T22:33:22.193384Z
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: enable remote Podman/bashy sandbox jobs so the local Podman VM can stay removed. Dependency: accept core dag -H source-sync/run/stream/fetch story e7f7175b65b7 first; do not weaken its gate. Before assigning a worker, probe novidesign.local, noviwin1, puppy for access, container runtime/version, available disk, and competing work; reserve one suitable host and record the probe. Then run a small declared DAG target that invokes bashy sandbox or Podman remotely in a host-owned test workspace. Acceptance: streamed logs and exit status, fetched artifact sha256, and second run transfers only changed Sources; record host/runtime/disk/command/delta. Confine container and storage to the test workspace and clean up only resources created by this test. If provider code is needed, implement it under this story with red/green tests and independent grade. Current known constraints: novidesign.local ~79 GiB free but no podman/docker in noninteractive PATH; noviwin1 and puppy noninteractive SSH request elevation, so capacity/runtime unverified. Preserve existing host data and system SSH service. Planned carryover if prerequisite or host reservation cannot be completed by 23:22:39Z.
