---
id: 5a917b8bc4d3
kind: feature
title: 'bashy peer channel = outpost''s in-process SSH (exec, SFTP, port-forward): wire it into bashy, no system ssh/sshd'
seq: 365
status: assigned
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:10.354723Z
weave: 13
assignee: codex-gpt5.6-terra
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: outpost already ships an in-process SSH server ('outpost sshd': shell, exec, SFTP/scp, port forwarding; no daemon, no pairing, no internet) and client ('outpost ssh'). REUSE it - do not build a new transport or trust model. Scope (3 pt): the remote bashy starts the embedded outpost SSH server (user-space, own port, key from the local bashy at install time); the local bashy uses the embedded outpost SSH client for exec, SFTP file sync and direct-tcpip/tcpip-forward. No dependency on the system ssh or sshd. Acceptance: red/green; e2e exec + file copy + one forwarded port dragon<->novidesign.local with the system sshd stopped.

PREREQUISITE VERIFIED 2026-09-30 (outpost 7775d56, story aabcde55 accepted): LAN PublicKeyCallback against authorized-keys file (SameUser guard, perms rejection, fingerprint-only logs) + 'outpost sshd --authorized-keys' flag + importable pkg/sshclient (Dial/Exec/SFTP/DirectTCPIP/LocalForward). Live proof: loopback key-auth with BatchMode, no password/TTY. Wire bashy to THESE (go.mod already replaces outpost? check; if not, add require+replace pinned to 7775d56 or newer). Key provisioning rides on remote-install (36f46b7e, merged 4332f32a): install lays the key, channel uses it. CONSTRAINTS: alternate outpost port; never stop system sshd; preserve novidesign.local user work. FILE PARTITION: new internal/agentos/peer_channel*.go only. Red/green tests; KISS.
