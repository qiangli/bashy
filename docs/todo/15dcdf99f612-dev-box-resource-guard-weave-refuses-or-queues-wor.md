---
id: 15dcdf99f612
kind: bug
title: 'Dev-box resource guard: weave refuses or queues work when memory, swap or disk run low; cap concurrent runs; keep heavy commands off the dev box'
seq: 3291
status: todo
priority: p0
created: 2026-11-21T21:41:34.194374Z
---

Incident 2026-11-21: the developer workstation (24 GB RAM, 461 GB disk) hit a kernel watchdog panic at 13:34 PST ('no checkins from watchdogd in 90 seconds'; compressor: 14 swapfiles, LOW swap space) and rebooted, killing every agent session and the attached gate-host run. Contributing causes (JetsamEvent 02:47 + panic report): fseventsd 14.0 GB (file-event churn from ~48 GB of weave workspaces), two govulncheck processes 13.4 + 7.6 GB run ON the dev box by an agent (dependency-bump story), a macOS Virtualization VM 8.6 GB (podman machine on the dev box, against the operator rule), many bashy agent processes 0.5-0.7 GB each, ~7 concurrent headless weave runs with Go builds launched by the conductor, and the disk at 96-99% full so swap could not grow (weave's own 2 GiB free-disk check fired at 13:16). Fix (smallest robust): (1) weave start refuses/queues when free memory (pressure), swap headroom or free disk is below thresholds, and caps concurrent headless runs per host (configurable, default small on workstations); (2) heavy commands known to exhaust a workstation (govulncheck over full graphs, full go test ./..., podman machine/VM start, sealed gates) are refused on a host marked as a dev box unless explicitly overridden with a logged reason, and point to the gate host; (3) weave prune of terminal clean workspaces runs automatically when disk crosses a threshold (delete-by-default, todo bf2f149d). Related: Sprint 688 spend/quota watermarks (49cd45d9). Acceptance: unit tests with injected memory/disk readings; a heavy command on a dev-box-marked host is refused with the override path shown.
