---
id: 295f6aecae48
kind: task
title: 'Klatch build: CGO_ENABLED=0 + windows/amd64 leg, cut v1.2.7, roll out native Klatch on Windows hosts'
seq: 3156
status: todo
priority: p1
labels:
    - klatch
    - windows
    - build
created: 2026-11-07T03:16:55.192711Z
sprint: 599
sprint_id: b38d7a89-cadd-58e2-8748-66b0e9c2f201
sprint_title: 'S585 follow-ups: native Klatch on the Windows test host, rc.30 end-to-end and UAT, local gates under 30 min'
---

The Klatch fork is pure Go as of commit febff05b (glebarez/sqlite over modernc; busy_timeout pragma verified by test; mattn/go-sqlite3 removed; CGO_ENABLED=0 builds for linux, darwin and windows; SSO team tests pass).
1. builds/klatch/build.yml and README: CGO_ENABLED=1 -> 0 on every leg; add a windows/amd64 leg plus its no-egress gate shape (README notes Windows was deferred for that reason).
2. Tag Klatch v1.2.7 at the fork, build and sign, then bump tool.env UPSTREAM_COMMIT and the yoke fleet pin.
3. Roll out on the Windows test host. Install the native service (bashy tool install klatch; bashy service add --optional klatch) on the same app name and port, with data from the existing container volume. A backup was taken 2026-11-07 (klatch-data volume export + databag.db copy). Verify SSO and that topic/message counts match the snapshot (accounts=4, topics=3, channels=3, cards=4, sso_identities=2). Then stop the container but keep it.
Closes S563 F2.

Progress (2026-11-07):
- Build change merged at bashy 2c77ea1a.
- Klatch v1.2.7 released on the fork; signed build bashy-v1.2.7-1 succeeded (linux, darwin and windows-amd64; the Windows no-egress runtime check reports NOT RUN).
- The yoke fleet pin is bumped at yoke 1b05a5ad (windows member klatch.exe).
Remaining:
- bashy must pin yoke >= 1b05a5ad and ship in the next rc, because `bashy tool install klatch` uses the fleet baseline compiled into bashy.
- Then switch the Windows test host to native Klatch, using the 2026-11-07 backup and snapshot counts.

Also required in this release (Sprint 565 O5): build from bashy main that includes outpost-profile up/down/status (examples/outpost-profile, bashy 5ad1d9ce or later), so the release source pin carries it.

## Release gate (operator, 2026-11-07)

No tag or release without the operator's explicit approval. Before asking for it:
1. Every umbrella submodule is clean, on its main branch and level with origin; the umbrella pins equal those heads (check with git submodule status: no + or -).
2. Work from every other agent and sprint is merged to the main branches or explicitly excluded with a reason (at wrap-up on 2026-11-07, Sprints 586, 589 and 591 were still open; the umbrella had an uncommitted AGENTS.md change from Sprint 591).
3. The candidate carries: outpost-profile up/down/status, satellite-member, bashy-defaults (apply/status/merge) with the first-install bootstrap step, kb reading kb/defaults from the source pin, the outpost register restart fix, and the Klatch v1.2.7 pin.
4. Then the gates (make test, test-bash 86/86 serial on a remote test host, GOWORK=off), QA, and the operator's go-ahead to tag.


## Release gate (operator, 2026-11-07)

No tag or release without the operator's explicit approval. Before asking for it:
1. Every umbrella submodule is clean, on its main branch and level with origin; the umbrella pins equal those heads (check with git submodule status: no + or -).
2. Work from every other agent and sprint is merged to the main branches or explicitly excluded with a reason (at wrap-up on 2026-11-07, Sprints 586, 589 and 591 were still open; the umbrella had an uncommitted AGENTS.md change from Sprint 591).
3. The candidate carries: outpost-profile up/down/status, satellite-member, bashy-defaults (apply/status/merge) with the first-install bootstrap step, kb reading kb/defaults from the source pin, the outpost register restart fix, and the Klatch v1.2.7 pin.
4. Then the gates (make test, test-bash 86/86 serial on a remote test host, GOWORK=off), QA, and the operator's go-ahead to tag.

## Progress (2026-11-08)

- Build part done. Klatch v1.2.7 (signed bashy-v1.2.7-1) is pinned by yoke 1b05a5ad; bashy main pins yoke f4dcd018, which contains it, so "bashy pins yoke >= 1b05a5ad" is met with no further sync. yoke main is ahead by one todo-only commit (593a238e), not needed for the release.
- bashy main also carries the build change (2c77ea1a) and outpost-profile up/down/status (5ad1d9ce).
- Remaining: cut the next rc from bashy main, run the gates and QA, tag and promote only with the operator approval, then switch the Windows test host to native Klatch from the 2026-11-07 backup. The Windows no-egress runtime check still reports NOT RUN.
