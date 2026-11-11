---
id: 01a1b809-6399-729b-8f2a-4e1f077c6b3b
seq: 21
form: page
type: runbook
title: 'Run the bashy tray: what it shows, prerequisites per OS, tray.yaml, install/uninstall autostart'
description: 'Optional desktop tray (examples/tray) showing outpost state with Admin and Apps windows in embedded webviews, plus bashy-tray install/uninstall for login autostart — WHEN a user wants a menu-bar/system-tray icon for outpost, a tray that starts at login, or onboard tray fails or skips (keywords: tray, bashy-tray, systray, autostart, login, LaunchAgent, Run key, autostart desktop, tray.yaml, webview, WebView2, WebKitGTK, AppIndicator, onboard tray)'
tags:
    - tray
    - outpost
    - autostart
    - macos
    - windows
    - linux
status: candidate
evidence: '2026-11-10: install/uninstall idempotence covered by examples/tray tests (files on darwin/linux, reg.exe on windows) and onboard_test.go; login-survival screenshots per OS still owed (sprint 605 T2)'
source:
    tool: claude
created: "2026-11-10T00:00:00Z"
---

## What the tray shows

An icon plus a status line, refreshed every ten seconds from `outpost status`:
green = outpost running and paired, amber = running but not paired, gray = off.
Menu items: Admin (outpost admin at `http://127.0.0.1:17777`, or `OUTPOST_ADMIN_ADDR`;
disabled when outpost is off), Apps (`http://127.0.0.1:22749/`; the tray starts
`bashy app serve` when nothing answers and stops it on Quit), Start/Stop outpost,
and any `tray.yaml` items. Windows open in embedded webviews, never the system
browser. Quit never stops outpost.

## Prerequisites per OS

`bashy-tray` is optional and built from `examples/tray` with cgo (`CGO_ENABLED=1 go build -o bashy-tray .`).
`bashy` and `outpost` must be on PATH.

| OS | Needs |
|---|---|
| macOS | Xcode Command Line Tools to build; WKWebView ships with the OS. Run in a logged-in desktop session. |
| Windows | C/C++ toolchain (MSYS2 MinGW-w64) to build with `-ldflags -H=windowsgui`; Microsoft WebView2 Evergreen Runtime. |
| Linux | `pkg-config`, GTK 3 dev headers, WebKitGTK **4.1**, and a tray host; stock GNOME needs the AppIndicator/KStatusNotifierItem extension. |

A missing runtime is an error; there is no browser fallback.

## tray.yaml shaping

Optional file `tray.yaml` in the bashy config dir (`$XDG_CONFIG_HOME/bashy`, else `~/.config/bashy`):

```yaml
items:
  - label: Local dashboard
    url: http://127.0.0.1:9000/
```

URLs must be absolute http/https without credentials. Read at startup: restart the tray after editing.

## Install / uninstall autostart

```sh
bashy-tray install      # start the tray at every login
bashy-tray uninstall    # remove it
```

Both are idempotent and say so when there is nothing to change. The entry points at the
resolved path of the running `bashy-tray`; rerun `install` after moving the binary.

| OS | Entry |
|---|---|
| macOS | `~/Library/LaunchAgents/com.qiangli.bashy-tray.plist` (RunAtLoad, Aqua session) |
| Windows | `HKCU\Software\Microsoft\Windows\CurrentVersion\Run` value `BashyTray` |
| Linux | `$XDG_CONFIG_HOME/autostart/bashy-tray.desktop` (default `~/.config/autostart`) |

`bashy onboard` (and `onboard tray`) calls `bashy-tray install` as its last step; when
`bashy-tray` is not on PATH it prints a skip line and carries on. The change takes effect
at the next login; to start now, just run `bashy-tray`.
