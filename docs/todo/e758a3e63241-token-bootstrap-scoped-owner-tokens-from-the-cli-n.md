---
id: e758a3e63241
kind: feature
title: 'token: bootstrap scoped owner tokens from the CLI (no browser), e.g. bashy token create --login'
seq: 3225
status: todo
priority: p1
created: 2026-11-11T21:19:24.064045Z
---

Sprint 613 (operator): minting a token with new scopes (members:*, hosts:*, satellites:read) needed the console in a browser: bashy token create can only mint a subset of the calling token's scopes, no existing token held them, and the console's scope list lacked hosts:* and satellites:read (cloudbox UI schema gap). Wanted: a CLI path from the owner's authentication to a scoped token, without a browser extension, e.g. (a) bashy token create --login: device/browser sign-in once (the same OAuth the console uses), then mint the requested scopes into the vault; or (b) an owner admin token (held only in the vault) that may mint any non-admin scope. Also: add hosts:read/hosts:write/satellites:read to the console scope list. KISS: (a) first.
