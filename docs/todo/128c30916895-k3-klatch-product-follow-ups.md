---
id: 128c30916895
kind: enhancement
title: K3 Klatch product follow-ups
seq: 3126
status: todo
priority: p2
labels:
    - klatch
created: 2026-11-06T18:13:56.660401Z
sprint: 577
sprint_id: d48e26ca-6b5d-50ba-a074-6af09d3fd177
sprint_title: 'Klatch follow-up: fleet release, outpost and satellite support, product limitations'
---

F2/F3 native Windows Klatch (pure-Go SQLite build; Bashy-provided sh/ffmpeg/ImageMagick) so it runs as a Bashy service without a container; F4/J5 host-owner setup as one Bashy command instead of the Python provisioner; F21 team join 12-15 s for 8 simultaneous first visits (serialized O(n) handshakes); F19 Enter does not send in the web client; Klatch PR #1 review notes (60 s replay window on loopback, reset token type reuse, test helpers addTestCard/openTestCard in production code, first-sign-in team owner until satellite roles exist). Carried from Sprint 563 (docs/plan-s563-master-execution.md). KISS/MVP: reuse existing code and harnesses; smallest causal repair. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.
