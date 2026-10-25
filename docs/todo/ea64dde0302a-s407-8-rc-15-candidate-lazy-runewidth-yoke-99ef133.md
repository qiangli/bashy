---
id: ea64dde0302a
kind: task
title: 'S407.8 rc.15 candidate: lazy runewidth (yoke 99ef1333) + lazy bonsai/x init (bonsai 4bb78b4) + startup gate; cut v1.0.0-rc.15-dev and replay Profile D'
seq: 1950
status: assigned
priority: p0
labels:
    - release
    - posix-cert
    - performance
created: 2026-10-25T04:44:32.670145Z
assignee: s407-manager
sprint: 407
sprint_id: 90546935-fbc2-5d09-94b0-19c380846681
sprint_title: bashy 1.0.0 release gates pending external dependencies (follow-up to Sprint 379)
---

Manager integration story for S407.6 621f736b (bashy 4e7ed4af merged as 265b7c15; runewidth fork published at yoke 99ef1333 after rebase of worker d4224e17) and S407.7 803503f3 (bonsai 3eec0fb merged as 4bb78b4). Gates: startup inittrace on Linux (cert base) and macOS (full build), make test-bash 86/86, go build. Then tag rc.15, verify assets + startup on the published binaries, new cert-approval 341-d tip, fresh-droplet Profile D replay (new worker: Muse quota-exhausted).
