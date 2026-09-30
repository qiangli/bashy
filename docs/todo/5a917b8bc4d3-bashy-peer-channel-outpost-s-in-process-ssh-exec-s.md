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
weave: 15
assignee: codex-gpt5.6-terra
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: outpost already ships an in-process SSH server ('outpost sshd': shell, exec, SFTP/scp, port forwarding; no daemon, no pairing, no internet) and client ('outpost ssh'). REUSE it. Scope (3 pt): remote bashy starts embedded outpost SSH server (user-space, own port, install-time key); local bashy uses embedded client for exec, SFTP, direct-tcpip/tcpip-forward. No system ssh/sshd. Acceptance: red/green; e2e exec + file copy + one forwarded port dragon<->novidesign.local.

PREREQUISITE VERIFIED (outpost 7775d56, aabcde55 accepted): LAN PublicKeyCallback + 'outpost sshd --authorized-keys' + importable pkg/sshclient; live loopback key-auth proven.

REWORK ROUND 2 (prior commit 0e1ffda adoptable from refs/salvage/abandoned-14, peer_channel.go 122 lines + test 50): the conductor could not verify it — close all four: (1) go.sum lacks outpost entries (workspace never built as submitted): run go mod tidy and COMMIT go.mod+go.sum. Ensure the workspace has ../outpost provisioned before gating. (2) Pin require to merged outpost main 7775d56, not branch commit 72e0497. (3) Standalone clones/CI have no ../outpost: wire outpost into .sibling-pins + scripts/bootstrap-siblings.sh per existing sibling pattern, or record the mechanism. (4) State EXACT measured gate numbers (build + Peer tests) in the commit message; e2e exec+file+forward on ALTERNATE port with key auth. CONSTRAINTS: never stop system sshd; preserve novidesign user work. PARTITION: peer_channel*.go + go.mod/go.sum/sibling-pins/bootstrap only. COMMIT everything; do not push.
