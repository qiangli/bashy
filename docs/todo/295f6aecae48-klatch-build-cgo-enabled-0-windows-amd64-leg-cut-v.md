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
---

The Klatch fork is pure Go as of commit febff05b (glebarez/sqlite over modernc; busy_timeout pragma verified by test; mattn/go-sqlite3 removed; CGO_ENABLED=0 builds for linux, darwin and windows; SSO team tests pass).
1. builds/klatch/build.yml and README: CGO_ENABLED=1 -> 0 on every leg; add a windows/amd64 leg plus its no-egress gate shape (README notes Windows was deferred for that reason).
2. Tag Klatch v1.2.7 at the fork, build and sign, then bump tool.env UPSTREAM_COMMIT and the yoke fleet pin.
3. Roll out on the Windows test host. Install the native service (bashy tool install klatch; bashy service add --optional klatch) on the same app name and port, with data from the existing container volume. A backup was taken 2026-11-07 (klatch-data volume export + databag.db copy). Verify SSO and that topic/message counts match the snapshot (accounts=4, topics=3, channels=3, cards=4, sso_identities=2). Then stop the container but keep it.
Closes S563 F2.
