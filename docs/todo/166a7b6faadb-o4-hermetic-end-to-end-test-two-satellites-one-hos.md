---
id: 166a7b6faadb
kind: test
title: 'O4 Hermetic end-to-end test: two satellites, one host''s outpost instances and Klatch in a sandbox, fake accounts'
seq: 3166
status: todo
priority: p0
labels:
    - outpost
    - klatch
    - e2e
created: 2026-11-07T11:18:34.201918Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: 'Simultaneous satellites: one outpost instance per satellite'
---

Replaces the O3 Linux leg. One run in a bashy oci (podman) sandbox on a Linux test host. It starts two throwaway satellites in dev mode, a host outpost paired with satellite A, a second instance paired with B through outpost-profile up, and Klatch from the pinned release with SSO and team mode. Accounts use example.test addresses only. The satellite-side harness is a private integration test in the hosted satellite repository, so public builds and CI never require it.

Checks through both satellites: anonymous refused; an account the app is not shared with refused; the owner and a shared member each get Klatch SSO with the same account GUID through A and B and see the same team topic; down on B leaves A serving. Exit contract: 0 pass, 1 fail, 2 BLOCKED (never a pass); one evidence record per candidate with exact Bashy, outpost, Klatch and satellite identities. The test droplet is destroyed after use. Deferred: message exchange through the Klatch API if it does not fit; a public fake-satellite layer for public CI (follow-up). KISS/MVP: reuse existing code and harnesses. Delivery commits carry Sprint, Story and Story-ID trailers.
