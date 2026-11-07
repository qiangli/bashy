---
id: 27eb793d78c1
kind: bug
title: 'T4 Satellite ticket: Google sign-in for a new account on zoo-aws ends in ''invalid state value'' (400) at the hosted portal callback'
seq: 3165
status: todo
priority: p3
labels:
    - satellite
created: 2026-11-07T10:19:54.967568Z
sprint: 565
sprint_id: 45140e29-11a7-5dac-b634-da49c692c9f6
sprint_title: 'Simultaneous satellites: one outpost instance per satellite'
---

Support ticket (cloudbox is proprietary; no shaping). Observed 2026-11-07 during S565 O3: in a fresh, isolated Chrome profile, a member account (Google Workspace account; Google reported hd=novigens.com) opened Klatch on zoo-aws (/matrix/h/elephant/app/klatch/), chose Sign in, and completed Google sign-in. The browser landed on the hosted portal callback (ai.dhnt.io/cloud/callback?state=/google-...&prompt=none), which answered 400 with 'invalid state value'; the console also showed a 404 for /favicon.ico. The owner account, already signed in to zoo-aws in another browser, is unaffected. Expected: the member is signed in to zoo-aws and returned to the app (the satellite then grants or refuses by share). Blocks the O3 member journey on zoo-aws. Not observed yet whether a retry, another provider, or the same account on zoo behaves the same.

Update 2026-11-07: a retry started from the zoo-aws home page in the same isolated profile signed the member in, and the satellite then correctly refused the unshared app ("Not shared with you"). Not reproduced; the first failure followed a silent re-auth (prompt=none) with a stale state. Downgraded to p3 and kept open only to watch for a recurrence; not sent as a ticket unless it repeats.
