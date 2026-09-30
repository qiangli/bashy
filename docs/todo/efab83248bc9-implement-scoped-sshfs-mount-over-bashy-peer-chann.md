---
id: efab83248bc9
kind: feature
title: Implement scoped SSHFS mount over Bashy peer channel
seq: 379
status: todo
priority: p1
created: 2026-09-30T22:37:42.236506Z
sprint: 344
sprint_id: e6708b39-af54-589c-94b7-0c18e222bf57
sprint_title: SSHFS over the Bashy peer channel for remote workspaces and sandboxes
---

After the feasibility story, implement a Bashy-managed FUSE mount for one authorized remote SFTP prefix over the existing peer key-auth channel. No password on command line. Cover read/write/rename/delete, path traversal protection, host-key trust, disconnect/reconnect, cancellation, idempotent unmount, and cleanup with focused tests. Limit host support to verified FUSE platforms; document other hosts.
