---
id: 030a968b059b
kind: test
title: 'O3 Verify simultaneous satellites: Klatch through two satellites on macOS, Linux and native Windows'
seq: 3060
status: todo
priority: p0
labels:
    - outpost
created: 2026-11-04T06:51:51.034687Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: Satellite-less single-node outpost by default; join and switch between satellites
---

Depends on O2. Main outpost paired with zoo and a second instance with zoo-aws on the same host. Klatch is reachable through both satellites; a member of each satellite signs in with SSO and both chat in the same team topic; down on the second instance leaves the main one working; after a reboot of the Windows test host both instances and the publication come back. Run on macOS (the dev box), a Linux test host and native Windows (elephant, claim it first). Keep raw logs and the exact Bashy, outpost and Klatch identities. Satellite-side gaps become support tickets. KISS/MVP: reuse existing code and harnesses; smallest causal change. Record exact source, build and artifact identities. Managers verify evidence before acceptance. Delivery commits carry Sprint, Story and Story-ID trailers.

## Progress (2026-11-07)

- macOS dev box (rc.27 pair 9187e1f0, main on zoo): `outpost-profile up zoo-aws --app o3echo` started outpost-zoo-aws (admin 17778, no LAN ssh, no mesh). A signed-in request through each satellite reached the one echo backend with the owner's identity and the same outpost HMAC signature (shared SSO secret, X-Forwarded-Host per satellite). Anonymous requests got 401 on both, with 0 backend hits. `down` removed the unit; zoo kept serving and zoo-aws went offline. Dev box restored afterwards. The live run found a bug, fixed in bashy 5ad1d9ce: up now declares sshd disabled in a fresh instance store.
- Native Windows (elephant, rc.27 pair 9187e1f0, main on zoo with the Klatch container): `outpost-profile up zoo-aws --app klatch` started outpost-zoo-aws under Bashy's supervisor (admin 17778, zoo-aws login success, publishing only klatch). The instance's klatch entry matches the main one (same SSO secret by hash). Through both satellites, Klatch POST /account/sso returned 200 and the owner landed in the same account and the same team topic (General, with the 2026-11-04 e2e history). The main outpost and the container were not touched. **The instance is left up on elephant** (`op.sh down zoo-aws` in the host's s565 folder removes it).
- Remaining: a member (non-owner) account on zoo-aws signing in and messaging the team; a Linux host; the Windows reboot check, deferred while the UAT cohort uses elephant. No test message was posted to the cohort's topic.
- Notes: app URLs are /matrix/h/<host>/app/<name>/. The Klatch web client shows the login view briefly before its SSO redirect lands on #/session. Remote PowerShell over outpost ssh loses all output when a statement errors (F22/F23 family); run native executables directly with forward-slash paths.
