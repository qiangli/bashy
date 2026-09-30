---
id: e7f7175b65b7
kind: feature
title: 'bashy dag -H <host>: run a target remotely as if local (sync Sources, run, stream, fetch Generates)'
seq: 366
status: todo
priority: p1
labels:
    - remote
created: 2026-09-30T17:19:12.148081Z
weave: 28
sprint: 342
sprint_id: bdacb510-6448-5851-acf3-7a19f6ccccb2
sprint_title: 'bashy dag remote: run any target on another host as if it were local (self-bootstrap, sync, run, fetch back)'
---

Owner 2026-09-30: extend dag's existing Host:/--mesh dispatch: for -H, sync the target's declared Sources (content-addressed, changed files only) into a stable remote workspace over the peer channel, run the same dag target there, stream stdout/stderr and exit status, copy declared Generates back. Acceptance: red/green; e2e 'bashy dag -H novidesign.local test' on a small repo; second run transfers only deltas.

Conductor 2026-09-30 (muse-spark1.3): PARKED behind owner decision on 5a917b8b (peer channel blocked, see mb:2500) — e2e needs the channel. When unblocked: build against yoke/pkg/dag SSHTransport (fleet_ssh.go) + exec_mesh.go dispatch; stable remote workspace must match the path remote-install manages; file partition: dag dispatch files + yoke dag extension only, no remote*.go/peer_channel*.go changes. Second-run-delta acceptance needs transfer logging in the test.

Conductor 2026-09-30 22:35Z: peer prerequisite 5a917b8b is accepted and pushed at Bashy 75ec2a0, with a real installed Bashy listener on novidesign.local:2223 and independent exec/SFTP/direct/forward proof. Resume this story now against that published base. Prior DAG run bashy#28 was killed during shared disk exhaustion; its staged 95-line argument-parser/test patch remains in the intact issue-28 workspace and is backed up at ~/.bashy/sprint/evidence/342/run28/staged.patch (sha256 258d07bdcd875ff1385d61411f7953a3341d9c961aec96aea8c9827f9fa2215d). Treat it as unverified reference work, not an accepted base. Keep Go compilation bounded (GOFLAGS=-p=1, GOMAXPROCS=2) and preserve raw failure output. The full gate still requires live source sync/run/stream/fetch on novidesign.local plus measured second-run delta. Remote sandbox/Podman dogfood is a separate dependent story 47518be4ebbd and must not dilute this acceptance. Box cutoff is 23:22:39Z; submit a committed candidate by 23:05Z for independent review or preserve partial work for carryover.
