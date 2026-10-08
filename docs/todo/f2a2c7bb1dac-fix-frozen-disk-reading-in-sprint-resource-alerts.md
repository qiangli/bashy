---
id: f2a2c7bb1dac
kind: bug
title: Fix frozen disk reading in sprint resource alerts
seq: 410
status: todo
priority: p1
labels:
    - sprint
created: 2026-10-08T17:22:53.940724Z
sprint: 399
sprint_id: 6648b5b1-c3d3-519e-b762-2b8e8af733ea
sprint_title: Stale disk alert shows peak reading after space reclaimed
---

Disk alert fired at the full-disk peak kept displaying the peak (e.g. 100.0%) after space was reclaimed: (1) activeSprintAlert Current message was frozen at fire time while the value sat in the 80-90 hysteresis band; (2) conditions unobserved for weeks were still served as live alerts. Fix in internal/agentos/sprint_monitor_alerts.go: refresh Current message/at on newer samples while active (no new notice, same ID; handoff notices preserved); activeSprintAlerts takes snapshot time and skips conditions unobserved over an hour. Regression tests: TestSprintMonitorDiskAlertTracksReclaimedSpace, TestSprintMonitorStaleAlertsAreNotServedAsLive (both fail pre-fix, pass post-fix); full go test -short ./internal/agentos/ green. Status: implemented and verified, uncommitted in bashy working tree.
