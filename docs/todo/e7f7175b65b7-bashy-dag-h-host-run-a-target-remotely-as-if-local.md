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
sprint: 402
sprint_id: 107fb1ff-6f9c-5bc1-8896-a26f792a6fd8
sprint_title: 'bashy dag post-1.0: reusable task graphs and remote execution'
---

Owner 2026-09-30: extend dag's existing Host:/--mesh dispatch: for -H, sync the target's declared Sources (content-addressed, changed files only) into a stable remote workspace over the peer channel, run the same dag target there, stream stdout/stderr and exit status, copy declared Generates back. Acceptance: red/green; e2e 'bashy dag -H novidesign.local test' on a small repo; second run transfers only deltas.

Conductor 2026-09-30 (muse-spark1.3): PARKED behind owner decision on 5a917b8b (peer channel blocked, see mb:2500) — e2e needs the channel. When unblocked: build against yoke/pkg/dag SSHTransport (fleet_ssh.go) + exec_mesh.go dispatch; stable remote workspace must match the path remote-install manages; file partition: dag dispatch files + yoke dag extension only, no remote*.go/peer_channel*.go changes. Second-run-delta acceptance needs transfer logging in the test.

Conductor 2026-09-30 22:35Z: peer prerequisite 5a917b8b is accepted and pushed at Bashy 75ec2a0, with a real installed Bashy listener on novidesign.local:2223 and independent exec/SFTP/direct/forward proof. Resume this story now against that published base. Prior DAG run bashy#28 was killed during shared disk exhaustion; its staged 95-line argument-parser/test patch remains in the intact issue-28 workspace and is backed up at ~/.bashy/sprint/evidence/342/run28/staged.patch (sha256 258d07bdcd875ff1385d61411f7953a3341d9c961aec96aea8c9827f9fa2215d). Treat it as unverified reference work, not an accepted base. Keep Go compilation bounded (GOFLAGS=-p=1, GOMAXPROCS=2) and preserve raw failure output. The full gate still requires live source sync/run/stream/fetch on novidesign.local plus measured second-run delta. Remote sandbox/Podman dogfood is a separate dependent story 47518be4ebbd and must not dilute this acceptance. Box cutoff is 23:22:39Z; submit a committed candidate by 23:05Z for independent review or preserve partial work for carryover.

Conductor 2026-09-30 23:03Z: bashy#30 commit 83bc7a8 is rejected for full acceptance but preserved at refs/salvage/abandoned-30 (also local branch s342-dag30-recovery). Its independent official grade PASS 69.446s build/focused tests; live novidesign small-repo streaming, fetch/hash, changed-only second run and unchanged third run PASS, raw ~/.bashy/sprint/evidence/342/dag30. Corrections required before merge: (1) `-f` outside the invoking root currently yields `../` and can write outside Bashy's remote workspace; support safe staging or fail closed before any remote write, with test; (2) a missing declared Source glob must not leave a stale remote copy silently; fail closed or synchronize deletion, with test; (3) `dag -H` must self-bootstrap a fresh remote through the accepted `remote install` path before dialing its peer channel, while preserving installed-host idempotence and key/host trust. Rebase candidate onto published Bashy origin/main 35a3aef and published coreutils/yoke pins. Submit a committed correction by about 23:15Z; independent full gate and host proof still required. No sandbox/SSHFS implementation in this run.

Review 2026-10-08 (moved from Sprint 345 to post-1.0 Sprint 402): dag -H is absent on bashy main; only --mesh/Host: dispatch exists (body over ssh, no Sources sync, no Generates fetch). The refs/salvage/abandoned-{27,28,30} refs and the s342-dag30-recovery branch are gone. The rejected candidate was preserved as patches: ~/.bashy/sprint/evidence/345/0001-feat-dag-sync-and-run-targets-over-peer-channel.patch (commit 83bc7a88337101d711fad3cc3739b9d287fc9697, sha256 e9aa7ee6929eec1406a62c2b7849cecc293e53a1acb288c2f0966b9ca42d5d14) and the run #9 remote-install scaffold ~/.bashy/sprint/evidence/345/0001-preserve-run-9-partial-work-remote-install-scaffold-.patch (ce1bdf984c1cb81e743643bbef6afd4fcdfe4f25, sha256 73a29966f3ea5fae75198a12a84ac9b55397305bd49ae636326024f302594a1f). Resume from the 83bc7a8 patch with its three recorded corrections. Source globs depend on 5a1e423d3801 (Sprint 379).
