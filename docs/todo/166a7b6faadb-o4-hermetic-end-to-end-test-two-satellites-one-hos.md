---
id: 166a7b6faadb
kind: test
title: 'O4 Hermetic end-to-end test: two satellites, one host''s outpost instances and Klatch in a sandbox, fake accounts'
seq: 3166
status: done
priority: p0
labels:
    - outpost
    - klatch
    - e2e
created: 2026-11-07T11:18:34.201918Z
assignee: claude-opus5.5-s565
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: 'Simultaneous satellites: one outpost instance per satellite'
closed: 2026-11-07T18:19:13.86721Z
closed_by: claude-opus5.5-s565
---

Replaces the O3 Linux leg. One run in a bashy oci (podman) sandbox on a Linux test host. It starts two throwaway satellites in dev mode, a host outpost paired with satellite A, a second instance paired with B through outpost-profile up, and Klatch from the pinned release with SSO and team mode. Accounts use example.test addresses only. The satellite-side harness is a private integration test in the hosted satellite repository, so public builds and CI never require it.

Checks through both satellites: anonymous refused; an account the app is not shared with refused; the owner and a shared member each get Klatch SSO with the same account GUID through A and B and see the same team topic; down on B leaves A serving. Exit contract: 0 pass, 1 fail, 2 BLOCKED (never a pass); one evidence record per candidate with exact Bashy, outpost, Klatch and satellite identities. The test droplet is destroyed after use. Deferred: message exchange through the Klatch API if it does not fit; a public fake-satellite layer for public CI (follow-up). KISS/MVP: reuse existing code and harnesses. Delivery commits carry Sprint, Story and Story-ID trailers.

## Delivered (2026-11-07)

The private integration test lives in the hosted satellite repository (dev branch). It runs both satellites, the host's two outpost instances (the second through `outpost-profile up`) and Klatch in one podman sandbox with no network, using fake example.test accounts. Two consecutive runs on an ephemeral Linux test droplet both passed 16/16 checks; the droplet was destroyed afterwards.

Checks: anonymous and unshared accounts refused through both satellites; owner and member get Klatch SSO through A and B with the same account per user; the owner's team topic contains the member and the member is connected to the owner; after `down` on B, A still serves and B no longer reaches the app.

Identities: Bashy and outpost v1.0.0-rc.27 (9187e1f0); Klatch 1.2.7 (signed build bashy-v1.2.7-1, sha256 matching the yoke pin); outpost-profile.bsh as of bashy 5ad1d9ce; satellite built in dev mode from the private repository's dev head at run time.

Test-only accommodations, which do not apply to separate machines: two satellites in one network namespace number host tunnel ports from the same base, so the test pairs a placeholder host on B first; the satellites' embedded cluster cannot start in the sandbox and is not needed for app access.

Not covered: real satellite Google sign-in and the sharing UI (manual), and Windows reboot (O3). Public fake-satellite layer for public CI: follow-up.
