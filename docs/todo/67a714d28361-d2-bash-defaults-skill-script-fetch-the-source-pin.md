---
id: 67a714d28361
kind: feature
title: 'D2 Bash# defaults skill/script: fetch the source pin, register default commands, report kb defaults'
seq: 3179
status: done
priority: p1
labels:
    - kb
created: 2026-11-07T22:07:52.602639Z
assignee: claude-opus5.5-s588
sprint: 588
sprint_id: 2ab4b70e-c274-5779-9e55-8259c38a4204
sprint_title: 'External defaults: kb pages and Bash# skill/scripts from the release source pin'
closed: 2026-11-07T22:23:47.374423Z
closed_by: claude-opus5.5-s588
---

One .bsh (command + ~~~skill fence) in bashy examples, e.g. examples/defaults/bashy-defaults.bsh: bashy src fetch bashy (or src upgrade), register each default example command (outpost-profile first) from the pin path with bashy commands add, and report whether kb/defaults is present for D1. Idempotent; never overwrites a user's own registration of the same name. The built-in bashy skill gets a short pointer to it (the only built-in). Tests with a fake bashy/outpost and temp BASHY_HOME. KISS/MVP: smallest change, reuse existing mechanisms (assetring shared ring, bashy src pin, bashy commands add). Public content uses generic names and variables for host/user/home; no operator details. Delivery commits carry Sprint, Story and Story-ID trailers.
