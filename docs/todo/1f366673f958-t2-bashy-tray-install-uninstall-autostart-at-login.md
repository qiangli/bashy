---
id: 1f366673f958
kind: feature
title: T2 bashy-tray install/uninstall autostart at login on 3 OSes; onboard tray calls it; kb page
seq: 3217
status: done
priority: p2
labels:
    - tray
    - windows
    - macos
    - linux
created: 2026-11-11T02:59:23.703527Z
weave: 92
assignee: claude-sonnet5.5
sprint: 605
sprint_id: 1d5a938b-6b62-5819-8451-d02151cdd908
sprint_title: 'Tray app (example level): outpost admin and bashy apps in embedded webviews'
closed: 2026-11-11T11:04:09.971238Z
closed_by: claude-opus5.5-s605
---

After T1. `bashy-tray install` / `uninstall`: macOS LaunchAgent plist, Windows HKCU Run key, Linux ~/.config/autostart .desktop; idempotent. Replace the stub `cmd_tray` in examples/onboard/onboard.bsh with a call to it (skip with a clear line when bashy-tray is not installed). One kb runbook page: what the tray shows, runtime prerequisites per OS, tray.yaml shaping, install/uninstall.

Verify: onboard_test.go updated; autostart survives logout/login on each OS (screenshot). Plan: dhnt-docs docs/sprint-605-master-execution-plan.md. Agent: claude sonnet5.5.
