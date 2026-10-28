---
id: 8acb0d5f4a3a
kind: feature
title: 'S522.7 outpost-profile add --name: per-profile host name so one machine can join several satellites'
seq: 2822
status: todo
priority: p0
labels:
    - pairing
    - profiles
created: 2026-10-28T10:40:57.60732Z
sprint: 522
sprint_id: 3413791d-7a40-5514-9970-99b64d1d83cf
sprint_title: 'AWS cloud integration and satellite hosting: bashy aws provider (Lightsail), budget guard, hosting research, self-hosted + hosted AWS satellites'
---

Operator 2026-10-28: one outpost registers with multiple satellites (work/home) and switches; do NOT move hosts. S514.5 finding 2 (umbrella origin/dev docs/sprint-514-acceptance-results.md): examples/outpost-profile/outpost-profile.bsh 'add' has no --name and always registers with the machine hostname, so a machine whose name is taken cannot add a profile. Change: add optional --name <host> (and --name=<host>) to 'add', passed through as 'outpost register --name'; default stays the hostname; 'list' shows each profile's server and host name; update the skill/help text. Tests: extend outpost_profile_test.go (fake OUTPOST_BIN records argv): --name forwarded, default unchanged, list shows name. Run only that package's tests. Pairs with cloudbox S522.6 (master accepts one assignment per satellite).
