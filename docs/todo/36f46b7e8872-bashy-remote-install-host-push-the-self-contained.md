---
id: 36f46b7e8872
kind: feature
title: 'bashy remote install <host>: push the self-contained bashy to a remote host if missing (host-OS bootstrap)'
seq: 364
status: assigned
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:08.373844Z
weave: 10
assignee: codex-gpt5.6-terra
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: bashy is self-contained, so the first step is to put bashy on the remote via whatever the host OS offers (ssh/scp on Unix hosts where available; the OS-native equivalent elsewhere), idempotently and version-matched (same release binary for the remote os/arch, from the local release cache - no internet needed on the remote). After this one bootstrap nothing depends on the remote's sshd. Acceptance (5 pt): red/green on a fake remote; e2e to novidesign.local; re-run is a no-op; version mismatch upgrades.

Conductor 2026-09-30 (muse-spark1.3): PRIOR PARTIAL WORK at refs/salvage/abandoned-9 (ce1bdf9): remote.go scaffold (576 lines) + remote_test.go (236 lines) + wiring in internal/agentos/agentos.go + commands.go. Adopt it, do not re-scaffold; verify with build+tests. FILE PARTITION: you own internal/agentos/remote*.go plus minimal wiring in agentos.go/commands.go only; do not touch peer-channel or dag files. REMOTE CONTRACT: novidesign.local is Darwin arm64, ssh-reachable, bashy absent, outpost present in ~/bin; preserve user work, touch only a stable workspace path. Local release cache: bashy internal/agentos/release.go + self_image.go (read-only reference). Red/green tests per story; KISS.
