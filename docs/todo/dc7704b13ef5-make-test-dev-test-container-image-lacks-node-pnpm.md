---
id: dc7704b13ef5
kind: bug
title: 'make test: dev-test-container image lacks node/pnpm, so test-meet-spa-fresh fails closed on every host'
seq: 3157
status: todo
priority: p1
labels:
    - test
    - ci
created: 2026-11-07T03:33:16.559352Z
---

Found on 2026-11-06 while gating v1.0.0-rc.27-dev. tools/dev-test-container/Containerfile (Ubuntu, golang-go only) has no node or pnpm. make test runs test-meet-spa-fresh as its first prerequisite, and that check exits 2 ('check requires node and pnpm ... but none is available'), so make test-self-container cannot pass anywhere. Fix: install pinned node plus pnpm (corepack) in the image, or provision them in the lane. Then re-run make test-self-container on the Windows test host's rootful gate machine.
