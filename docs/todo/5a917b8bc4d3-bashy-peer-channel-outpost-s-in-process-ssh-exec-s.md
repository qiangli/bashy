---
id: 5a917b8bc4d3
kind: feature
title: 'bashy peer channel = outpost''s in-process SSH (exec, SFTP, port-forward): wire it into bashy, no system ssh/sshd'
seq: 365
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:10.354723Z
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: outpost already ships an in-process SSH server ('outpost sshd': shell, exec, SFTP/scp, port forwarding; no daemon, no pairing, no internet) and client ('outpost ssh'). REUSE it - do not build a new transport or trust model. Scope (3 pt): the remote bashy starts the embedded outpost SSH server (user-space, own port, key from the local bashy at install time); the local bashy uses the embedded outpost SSH client for exec, SFTP file sync and direct-tcpip/tcpip-forward. No dependency on the system ssh or sshd. Acceptance: red/green; e2e exec + file copy + one forwarded port dragon<->novidesign.local with the system sshd stopped.
