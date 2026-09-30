---
id: 2b0ded7bbd9f
kind: test
title: Dogfood SSHFS with remote sandbox and artifact return
seq: 380
status: todo
priority: p2
created: 2026-09-30T22:37:42.324959Z
sprint: 344
sprint_id: e6708b39-af54-589c-94b7-0c18e222bf57
sprint_title: SSHFS over the Bashy peer channel for remote workspaces and sandboxes
---

After Sprint 342 dag -H core gate and the SSHFS mount story, reserve a verified remote host with FUSE and Podman or bashy sandbox. Mount a sprint-owned test directory, run a container target against it, verify streamed exit/logs and artifact hash, repeat incrementally, then unmount and remove only test resources. Record exact host/runtime/disk and evidence; no local Podman VM.
