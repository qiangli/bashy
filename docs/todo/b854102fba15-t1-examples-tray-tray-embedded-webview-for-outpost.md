---
id: b854102fba15
kind: feature
title: 'T1 examples/tray: tray + embedded webview for outpost admin and bashy apps on Windows, macOS, Linux'
seq: 3216
status: assigned
priority: p1
labels:
    - tray
    - windows
    - macos
    - linux
created: 2026-11-11T02:59:23.538476Z
weave: 80
assignee: codex-gpt6-sol
sprint: 605
sprint_id: 1d5a938b-6b62-5819-8451-d02151cdd908
sprint_title: 'Tray app (example level): outpost admin and bashy apps in embedded webviews'
---

Example level: own go.mod under examples/tray, never linked into the core binary (cgo allowed). fyne.io/systray + webview/webview_go (WebView2 / WKWebView / WebKitGTK); no system browser. Each window is a child process (`bashy-tray window URL TITLE`); re-selecting an open item focuses it.

Menu: status line; Admin -> outpost admin UI (http://127.0.0.1:17777, $OUTPOST_ADMIN_ADDR); Apps -> bashy app (http://127.0.0.1:22749/; start `bashy app serve` as a child only if nothing answers, stop it on Quit); Start/Stop outpost (existing verbs); Quit (never stops outpost).

Status, 10 s poll: `outpost status --local-presence` + `outpost status --json` (.status.configured/.state): on = running+paired, partial = running+unpaired, off = not running (Admin disabled, Start offered).

Shaping hook: optional tray.yaml in the bashy config dir, extra {label,url} items opened in a webview. Missing webview runtime / tray host (WebKitGTK 4.1, GNOME AppIndicator, WebView2) fails with a clear message; never a browser fallback. README: tech note + per-OS build steps. No changes to yoke, outpost or core bashy.

Verify: targeted tests; smoke per OS (macOS dev box, Windows host, Linux desktop) with screenshots. Plan: dhnt-docs docs/sprint-605-master-execution-plan.md. Agent: codex gpt6-sol.
