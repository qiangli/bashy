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

Owner 2026-09-30: bashy must reuse outpost's in-process SSH server and public client, with no system ssh/sshd after host-OS bootstrap. Full acceptance: remote Bashy starts the embedded server on a LAN-reachable alternate port using an install-time key; local Bashy uses embedded client for key-authenticated exec, SFTP file copy, direct-tcpip and one forwarded port. Prove all four dragon ↔ novidesign.local on real hosts. Preserve user work and system sshd.

VERIFIED PREREQUISITES: outpost LAN key auth/public pkg/sshclient 7775d56; public pkg/sshserver merged a45f190, pushed in outpost main d3d23b0 and umbrella pin b238958b. Both have independent gates. Candidate refs/salvage/abandoned-25 (4237033 + fb840e2) contains a buildable in-process server/client adapter and loopback tests; adopt it, then finish remote startup. Do not adopt refs/salvage/abandoned-22 external `outpost sshd` process; it was rejected.

REQUIRED REMAINING WORK: (1) During `bashy remote install`, provision persistent Bashy-owned client private/public key, remote authorized_keys, and remote host key with appropriate permissions, without overwriting unrelated files. (2) Make the installed remote Bashy start/serve `outpost/pkg/sshserver` in its own process on LAN port 2223; local Bashy dials via `outpost/pkg/sshclient` with the installed key and a verified/pinned host key. Expose only minimal CLI/wiring needed; `peer_channel*.go`, `remote*.go`, and minimal command wiring are allowed. (3) Independent local red/green tests plus a real dragon↔novidesign.local exec, SFTP roundtrip and forwarded-port echo on alternate port. Show command/output/exit evidence. A loopback-only test or external outpost process is partial and must not be submitted as done. (4) Commit go.mod/go.sum, preserve `.sibling-pins`/bootstrap for standalone clones, state exact gate results. Do not push. No system sshd stop, no user workspace modification.

Gate: `go build ./... && go test ./internal/agentos -run "PeerChannel|PeerSSHServer|Remote" -count=1 && ./scripts/test-sibling-pins.sh`, plus the live novidesign e2e. Use serial GOFLAGS=-p=1 if host disk is constrained.
