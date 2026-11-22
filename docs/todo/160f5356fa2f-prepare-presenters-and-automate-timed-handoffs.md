---
id: 160f5356fa2f
kind: feature
title: Prepare presenters and automate timed handoffs
seq: 3294
status: done
priority: p1
labels:
    - demo-queue
created: 2026-11-21T23:44:29.648583Z
assignee: s690-codex-manager
sprint: 690
sprint_id: 3462c15d-bc37-505c-849d-64a3453232d7
sprint_title: Demo Queue — LAN screen sharing for team demos
closed: 2026-11-22T09:58:24.995906Z
closed_by: s690-codex-manager
---

Build on the Outpost media story in this sprint. Joining adds a roster entry. Prepare to present opens the picker, shows a private local preview, and checks browser encoding support, a live capture source, and a bounded synthetic upload over the media path. On success, queue presenters in ready order and show a green Ready status to the organizer. Show a retry action on failure. Use session heartbeats and capture-track events to clear readiness on disconnect, withdrawal, or stopped capture. Reconnection keeps roster identity and requires preparation checks before reentry.

The organizer can Start, Skip, Assign turn, and End session. Automatically start sharing when called. Default to a 120-second turn and a 15-second warning, configurable before Start. The host owns turn IDs and deadlines and starts the timer on the first rendered projector frame. Use a small red pulse during the warning and steady red with reduced motion. Clear picture and sound at expiry, finish, skip, or reassignment, then start the next ready presenter. Advance on presenter disconnect or failure to appear within 10 seconds and surface the reason. Wait and resume when the queue is empty.

Assign a ready presenter immediately. For someone who needs to prepare, prompt them and give them priority once their checks pass while the current demo continues. Requeue an interrupted presenter at the front if their source and connection remain healthy. Recalled presenters get a fresh timer. A returning projector gets a fresh media stream with the same deadline. Release capture on completion, withdrawal, or connection loss.

Acceptance
- Cover prepare and retry, green readiness, ready order, stale readiness, and local preview without capture upload before assignment.
- Cover first-frame timing, warning and expiry, early finish, skip, recall, disconnect, rejected stale media, and end-session cleanup. Confirm picture and audio stop on every handoff.
