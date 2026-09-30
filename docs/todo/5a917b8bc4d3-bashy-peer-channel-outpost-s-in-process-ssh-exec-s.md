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

Owner 2026-09-30: Bashy must use outpost's in-process SSH server and public client. Full acceptance stays: install-time key auth and pinned host key; remote installed Bashy itself serves LAN port 2223; local Bashy uses its embedded outpost client for exec, SFTP, direct-tcpip, and one forwarded port; prove all four dragon <-> novidesign.local. No system sshd after first host-OS bootstrap; preserve unrelated user work and system sshd.

READY PREREQUISITES: outpost public pkg/sshclient and pkg/sshserver accepted and pushed d3d23b0; umbrella pin b238958b. Bashy refs/salvage/abandoned-26 contains three clean commits 25f2647, d2e6808, d5b3483: buildable peer adapter, hidden `remote peer-serve`, focused loopback tests, sibling pin. Cherry-pick all three onto current bashy main. Do not use rejected external `outpost sshd` subprocess from abandoned-22.

REMAINING IMPLEMENTATION, IN THIS ORDER:
1. Extend `runRemoteInstall` in internal/agentos/remote.go after binary verification. Persist Bashy-owned local client ed25519 private/public key and expected remote host public key under ~/.bashy/remote/peers/<host>/ with 0700 parent, 0600 private. Persist remote authorized_keys and host private key under remote ~/.bashy/remote/ with 0700 parent, 0600 private. Generate keys in Go and install through the existing host-OS bootstrap transport; no overwrites of unrelated files. Rerun must preserve existing keys and work even if binary version matches. Fake-root tests must prove idempotence and permissions.
2. Start installed `~/.bashy/remote/bin/bashy remote peer-serve --authorized-keys ... --host-key ...` in its OWN persistent process on LAN port 2223. First install may use host OS ssh/scp to launch; after it succeeds, subsequent peer operations must use the embedded channel, not OS ssh. Do not start an external outpost process or stop system sshd. Make startup idempotent; detect an existing listener and verify its expected host key, never kill unrelated processes.
3. Wire local peer dial to use persisted client key plus ssh.FixedHostKey from the pinned expected remote host public key. Provide minimal Bashy CLI or exported path to exercise exec, SFTP roundtrip, direct-tcpip and local forward through the embedded client. Fail closed on missing or changed host key.
4. Run red/green tests and a real dragon <-> novidesign.local proof: command/output/exit for exec, SFTP upload/download to a Bashy-owned temp path, direct-tcpip echo, local forwarded-port echo on alternate port. The OS bootstrap may use system ssh once; live proof must work with OS sshd unavailable to the Bashy peer path. Do not alter unrelated remote files. Commit all source/go.mod/go.sum/pin changes with Sprint/Story trailers. Do not push.

Required gate: GOFLAGS=-p=1 GOMAXPROCS=2 go build ./... && GOFLAGS=-p=1 GOMAXPROCS=2 go test ./internal/agentos -run "PeerChannel|PeerSSHServer|Remote" -count=1 && ./scripts/test-sibling-pins.sh, plus live proof. Use the full bounded run for the missing work; if blocked, commit work and report exact remaining items without claiming acceptance. No loopback-only completion.
