---
id: 36f46b7e8872
kind: feature
title: 'bashy remote install <host>: push the self-contained bashy to a remote host if missing (host-OS bootstrap)'
seq: 364
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:08.373844Z
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: bashy is self-contained, so the first step is to put bashy on the remote via whatever the host OS offers (ssh/scp on Unix hosts where available; the OS-native equivalent elsewhere), idempotently and version-matched (same release binary for the remote os/arch, from the local release cache - no internet needed on the remote). After this one bootstrap nothing depends on the remote's sshd. Acceptance (5 pt): red/green on a fake remote; e2e to novidesign.local; re-run is a no-op; version mismatch upgrades.
