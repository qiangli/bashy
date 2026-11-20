---
id: f03e724d00ad
kind: feature
title: 'tray: Chat item opens the same chat/meet conversation as the browser extension'
seq: 3256
status: todo
priority: p2
created: 2026-11-20T14:33:55.879887Z
---

Operator 2026-11-20: add 'Chat' as one item in the tray (bashy examples/tray), opening the SAME conversation the browser extension's Shape posts into (yoke story a79cefd4 step 2: the agent joins the chat/meet room; the popup shows it). One conversation, many surfaces: TUI/chat, browser extension, tray.
Constraints (S605 tray design rule): the tray only watches — it opens the room in its embedded webview (like Admin/Apps); it never starts meet serve, the browser hub or any unit; when the room/server is not reachable it shows that state instead. macOS and Windows parity (Linux deferred).
Depends on: a79cefd4 step 2 (room view) — do after step 1 lands.
Acceptance: tray > Chat opens the conversation with the agent in a webview on macOS and Windows; messages sent from the extension's Shape appear there with their tab; nothing is spawned by the tray.
