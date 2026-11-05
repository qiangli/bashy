---
id: 9f27d68da431
kind: test
title: A4a Public lanes on the frozen candidate f1bcd711
seq: 3061
status: done
priority: p1
labels:
    - clean-baseline
created: 2026-11-04T07:49:13.683305Z
weave: 18
assignee: muse-conductor
sprint: 561
sprint_id: 3d035bd3-fa1c-5b4d-a3cc-386b0ec789a4
sprint_title: 'Foundation A: current-baseline repairs and POSIX certification readiness'
closed: 2026-11-04T19:37:09.301318Z
closed_by: claude-opus5.5
---

Child of A4 (12861d60bc4f), filed for independent ownership; A4 acceptance unchanged. FROZEN CANDIDATE: bashy f1bcd711 (sh 74586fde, coreutils 211789fa), standalone GOWORK=off. Run ONCE each, existing bounds, no skip/timeout growth, no code changes (failures are reported, not fixed): (1) GOWORK=off go test ./... full (per-package ok/FAIL/no-test; classify each FAIL vs A2 baseline docs/s561/a2-results.md: fixed / still-pre-existing / NEW); (2) GOWORK=off make dist (6 platforms x 2 entries, record sha256); (3) GOWORK=off sh scripts/s561-baseline-manifest.sh f1bcd711 after GOWORK=off make build -> docs/s561/candidate-manifest.json; (4) make test-bash-container if a container runtime (podman/docker or bashy podman) is usable, else record the exact setup blocker and native make test-bash 86/86 as non-authoritative. Disk is tight: clean only your own artifacts. Deliver docs/s561/a4a-results.md + candidate-manifest.json. Commit trailers Sprint: #561, Story: <seq>, Story-ID: <id>; BASHY_AGENT = you. Do not push.
