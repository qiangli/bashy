---
id: 2103beb6f8b5
kind: bug
title: 'viewer: embedded video assets render a player that never plays'
seq: 3203
status: todo
priority: p2
labels:
    - viewer
    - desk
created: 2026-11-09T15:59:17.762025Z
---

Repro (2026-11-09): `bashy viewer add`, then `viewer append BOOK -f page.md --asset demo.mp4` where page.md embeds `![](demo.mp4)`. The page renders `<video class="desk-embed desk-embed-video" controls preload="none" src="/assets/BOOK/1/demo.mp4">` but it shows 0:00 with no frame, and pressing play (click, Space or Enter on the focused button) never starts playback. The same MP4 (H.264/AAC, 1920x1080, 87 s, ~1 MB) plays in a plain Chrome tab.

Server side looks correct: GET /assets/BOOK/1/demo.mp4 returns 200 video/mp4, and a Range request returns 206 with Content-Range. Suspect the asset response's `Content-Security-Policy: default-src 'none'; sandbox; frame-ancestors 'self'` (examples/desk/viewer/main.go asset handler) or the page CSP (kit.go) blocking media; not yet confirmed (the page CSP also forbids eval, so it could not be probed from devtools automation).

Expected: a video asset embedded in a page plays in the viewer, with a poster/first frame (consider preload="metadata"), and in exported HTML. Add a verifydom/browser test that an embedded mp4 reaches readyState >= 1 and plays.

Motivation: showing a recorded demo video in the viewer instead of opening the file directly.
